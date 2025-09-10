package staff

import (
	"fmt"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

func DashboardHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// placeholder for now
		userID := sm.GetString(r.Context(), "user_id")
		email := sm.GetString(r.Context(), "email")
		w.Write([]byte(fmt.Sprintf("Dashboard for user %s with email %s", userID, email)))
	}
}
