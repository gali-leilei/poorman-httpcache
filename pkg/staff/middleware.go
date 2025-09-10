package staff

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

func AuthorizeMiddleware(sm *scs.SessionManager, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			token := sm.GetString(r.Context(), "authorized")
			if token != "true" {
				logger.Info("Unauthorized request", "url", path)
				http.Redirect(w, r, "/auth/request", http.StatusSeeOther)
				return
			}
			userID := sm.GetString(r.Context(), "user_id")
			email := sm.GetString(r.Context(), "email")
			logger.Info("Authorized request", "url", path, "user_id", userID, "email", email)
			next.ServeHTTP(w, r)
		})
	}
}
