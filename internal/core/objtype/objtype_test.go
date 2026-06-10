package objtype

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	reg := Register(Type{Key: "test.widget", Name: "Widget", Plural: "Widgets"})
	if reg.Key != "test.widget" {
		t.Fatalf("Register returned %+v", reg)
	}

	got, ok := Get("test.widget")
	if !ok || got.Plural != "Widgets" {
		t.Fatalf("Get(test.widget) = %+v, %v", got, ok)
	}

	if _, ok := Get("test.missing"); ok {
		t.Fatal("Get(test.missing) found an unregistered type")
	}

	all := All()
	found := false
	for _, x := range all {
		if x.Key == "test.widget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("All() missing test.widget: %+v", all)
	}
}

func TestRegisterPanics(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected panic", name)
			}
		}()
		fn()
	}

	mustPanic("uppercase", func() {
		Register(Type{Key: "Test.Widget", Name: "x", Plural: "x"})
	})
	mustPanic("no domain", func() {
		Register(Type{Key: "widget", Name: "x", Plural: "x"})
	})
	mustPanic("underscore", func() {
		Register(Type{Key: "test.device_type", Name: "x", Plural: "x"})
	})
	mustPanic("missing names", func() {
		Register(Type{Key: "test.nameless"})
	})

	// Multi-word parts use hyphens, mirroring slug syntax.
	Register(Type{Key: "test.device-type", Name: "Device type", Plural: "Device types"})

	mustPanic("duplicate", func() {
		Register(Type{Key: "test.device-type", Name: "x", Plural: "x"})
	})
}
