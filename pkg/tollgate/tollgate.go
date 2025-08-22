package tollgate

import "net/http"

type Tollgate struct {
	extractKey func(r *http.Request) string
	adapter    Adapter
}

func New(adapter Adapter, keyFunc func(r *http.Request) string) *Tollgate {
	return &Tollgate{adapter: adapter, extractKey: keyFunc}
}

func (t *Tollgate) HTTPHandlerMiddleware(next http.Handler) http.Handler {
	return &tollgateHTTPHandler{
		next:   next,
		client: t,
	}
}

type tollgateHTTPHandler struct {
	next   http.Handler
	client *Tollgate
}

func (h *tollgateHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := h.client.extractKey(r)
	balance, err := h.client.adapter.Balance(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if balance <= 0 {
		http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
		return
	}

	h.next.ServeHTTP(w, r)
	// TODO: check if the request is successful. only consume if the request is successful.
	h.client.adapter.Consume(r.Context(), key)
}
