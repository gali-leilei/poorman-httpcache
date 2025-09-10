// Package staff provides the staff dashboard handler, using session to
package staff

import (
	"httpcache/pkg"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

func NewRouter(sm *scs.SessionManager, allowList AllowlistFunc, sendMail SendMailFunc, form *Form, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(pkg.GetLoggerMiddleware(logger))
	router.Use(middleware.Recoverer)

	router.Route("/auth", func(r chi.Router) {
		nextPath := "/dashboard"
		r.Use(sm.LoadAndSave)
		r.Get("/request", ShowVerificationFormHandler(form, logger))
		r.Post("/request", RequestVerificationHandler(sm, form, allowList, sendMail, nextPath, logger))
		r.Get("/verify", CompleteVerificationHandler(sm, nextPath))
		r.Get("/logout", LogoutHandler(sm))
	})
	router.Route("/dashboard", func(r chi.Router) {
		r.Use(sm.LoadAndSave)
		r.Use(AuthorizeMiddleware(sm, logger))
		r.Handle("/*", DashboardHandler())
	})

	return router
}
