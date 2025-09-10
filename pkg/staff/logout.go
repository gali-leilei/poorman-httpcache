package staff

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
)

func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sm.Destroy(r.Context())
		w.Write([]byte("Logged out"))
	}
}
