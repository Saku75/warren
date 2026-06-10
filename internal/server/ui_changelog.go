package server

import (
	"net/http"

	"github.com/saku75/warren/internal/web"
)

func (u *uiHandler) changelogPage(w http.ResponseWriter, r *http.Request) {
	items, total, err := u.changelog.List(r.Context(), uiListLimit, 0)
	if err != nil {
		status, msg := u.fail(r, err)
		u.render(w, r, status, web.ErrorPage(msg))
		return
	}
	u.render(w, r, http.StatusOK, web.ChangelogPage(items, total))
}
