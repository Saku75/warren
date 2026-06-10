package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/dbtest"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/tenancy"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	pool := dbtest.Pool(t)
	h := New(
		slog.New(slog.DiscardHandler),
		tenancy.NewService(pool),
		dcim.NewService(pool),
		changelog.NewService(gen.New(pool)),
	)
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv
}

// call sends a JSON request and decodes the JSON response.
func call(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	out := map[string]any{}
	if resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	}
	return resp.StatusCode, out
}

func uniq(prefix string) string {
	u := id.New().String()
	return fmt.Sprintf("%s-%s", prefix, u[len(u)-12:])
}

func TestAPIEndToEnd(t *testing.T) {
	srv := testServer(t)

	// Tenant create with explicit slug.
	tenantSlug := uniq("acme")
	status, body := call(t, srv, "POST", "/tenancy/tenants", map[string]string{
		"name": "Acme", "slug": tenantSlug,
	})
	if status != http.StatusCreated {
		t.Fatalf("create tenant: %d %v", status, body)
	}
	if body["type"] != "tenancy.tenant" || body["slug"] != tenantSlug {
		t.Fatalf("tenant rep wrong: %v", body)
	}

	// Site create referencing the tenant by slug.
	siteSlug := uniq("cph-dc")
	status, body = call(t, srv, "POST", "/dcim/sites", map[string]string{
		"name": "Copenhagen DC", "slug": siteSlug, "tenant": tenantSlug, "status": "staging",
	})
	if status != http.StatusCreated {
		t.Fatalf("create site: %d %v", status, body)
	}
	siteID := body["id"].(string)
	tenant, _ := body["tenant"].(map[string]any)
	if tenant == nil || tenant["slug"] != tenantSlug {
		t.Fatalf("site tenant wrong: %v", body)
	}

	// Fetch by slug and by ID must agree.
	status, bySlug := call(t, srv, "GET", "/dcim/sites/"+siteSlug, nil)
	if status != http.StatusOK || bySlug["id"] != siteID {
		t.Fatalf("get by slug: %d %v", status, bySlug)
	}
	status, byID := call(t, srv, "GET", "/dcim/sites/"+siteID, nil)
	if status != http.StatusOK || byID["slug"] != siteSlug {
		t.Fatalf("get by id: %d %v", status, byID)
	}

	// Nested locations via path refs.
	status, body = call(t, srv, "POST", "/dcim/locations", map[string]string{
		"site": siteSlug, "name": "Building A", "slug": "building-a", "kind": "building",
	})
	if status != http.StatusCreated {
		t.Fatalf("create building: %d %v", status, body)
	}
	status, body = call(t, srv, "POST", "/dcim/locations", map[string]string{
		"site": siteSlug, "parent": "building-a", "name": "Floor 1", "slug": "floor-1", "kind": "floor",
	})
	if status != http.StatusCreated {
		t.Fatalf("create floor: %d %v", status, body)
	}
	wantPath := siteSlug + "/building-a/floor-1"
	if body["slug_path"] != wantPath {
		t.Fatalf("slug_path = %v, want %s", body["slug_path"], wantPath)
	}

	// Resolve by full slug path.
	status, body = call(t, srv, "GET", "/dcim/locations/"+wantPath, nil)
	if status != http.StatusOK || body["slug"] != "floor-1" {
		t.Fatalf("get location by path: %d %v", status, body)
	}
	parent, _ := body["parent"].(map[string]any)
	if parent == nil || parent["slug"] != siteSlug+"/building-a" {
		t.Fatalf("parent rep wrong: %v", body)
	}

	// Duplicate slug in the same scope conflicts; same slug in another
	// scope is fine.
	status, body = call(t, srv, "POST", "/dcim/locations", map[string]string{
		"site": siteSlug, "parent": "building-a", "name": "Floor 1 dup", "slug": "floor-1",
	})
	if status != http.StatusConflict {
		t.Fatalf("dup location: %d %v", status, body)
	}
	status, _ = call(t, srv, "POST", "/dcim/locations", map[string]string{
		"site": siteSlug, "name": "Floor 1 at root", "slug": "floor-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("same slug different scope: %d", status)
	}

	// Location tree endpoint.
	status, body = call(t, srv, "GET", "/dcim/sites/"+siteSlug+"/locations", nil)
	if status != http.StatusOK {
		t.Fatalf("site locations: %d %v", status, body)
	}
	items := body["items"].([]any)
	if len(items) != 2 { // building-a and root floor-1
		t.Fatalf("tree roots = %d, want 2: %v", len(items), body)
	}

	// PATCH semantics: rename without touching slug.
	status, body = call(t, srv, "PATCH", "/dcim/sites/"+siteSlug, map[string]string{
		"name": "Copenhagen DC 1",
	})
	if status != http.StatusOK || body["slug"] != siteSlug || body["name"] != "Copenhagen DC 1" {
		t.Fatalf("patch site: %d %v", status, body)
	}

	// Tenant deletion blocked while referenced.
	status, body = call(t, srv, "DELETE", "/tenancy/tenants/"+tenantSlug, nil)
	if status != http.StatusConflict {
		t.Fatalf("tenant delete while referenced: %d %v", status, body)
	}

	// Changelog captured the object history.
	status, body = call(t, srv, "GET", "/changelog?object_type=dcim.site&object_id="+siteID, nil)
	if status != http.StatusOK {
		t.Fatalf("changelog: %d %v", status, body)
	}
	events := body["items"].([]any)
	if len(events) != 2 { // create + update
		t.Fatalf("site changelog events = %d, want 2: %v", len(events), body)
	}

	// Cleanup path: site delete cascades, then tenant delete works.
	if status, body = call(t, srv, "DELETE", "/dcim/sites/"+siteSlug, nil); status != http.StatusNoContent {
		t.Fatalf("delete site: %d %v", status, body)
	}
	if status, body = call(t, srv, "DELETE", "/tenancy/tenants/"+tenantSlug, nil); status != http.StatusNoContent {
		t.Fatalf("delete tenant: %d %v", status, body)
	}
	if status, body = call(t, srv, "GET", "/dcim/locations/"+wantPath, nil); status != http.StatusNotFound {
		t.Fatalf("location after site delete: %d %v", status, body)
	}

	// Unknown endpoint returns the JSON error shape.
	status, body = call(t, srv, "GET", "/nope", nil)
	if status != http.StatusNotFound || body["error"] == nil {
		t.Fatalf("not found shape: %d %v", status, body)
	}
}
