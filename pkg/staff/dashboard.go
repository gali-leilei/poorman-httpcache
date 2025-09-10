package staff

import "net/http"

func DashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// placeholder for now
		w.Write([]byte("Dashboard"))
	}
}
