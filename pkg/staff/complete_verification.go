package staff

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
)

func CompleteVerificationHandler(sm *scs.SessionManager, nextPath string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		// check if user is already logged in
		isAuthorized := sm.GetString(r.Context(), "authorized")
		if isAuthorized == "true" {
			http.Redirect(w, r, nextPath, http.StatusSeeOther)
			return
		}

		// get magic link from query params
		magicLink := r.URL.Query().Get("onetime_token")
		if magicLink == "" {
			http.Error(w, "Magic link is required", http.StatusBadRequest)
			return
		}

		// get magic link from session manager
		cachedLink := sm.PopString(r.Context(), "onetime_token")
		if cachedLink != magicLink {
			http.Error(w, "Invalid magic link", http.StatusBadRequest)
			return
		}

		sm.Put(r.Context(), "authorized", "true")
		sm.RenewToken(r.Context())
		http.Redirect(w, r, nextPath, http.StatusSeeOther)
	}
}
