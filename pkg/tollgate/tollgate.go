package tollgate

import (
	"log/slog"
	"net/http"
)

type Tollgate struct {
	extractKey func(r *http.Request) string
	adapter    Adapter
	logger     *slog.Logger
}

func New(adapter Adapter, keyFunc func(r *http.Request) string, logger *slog.Logger) *Tollgate {
	ctxLogger := logger.With("middleware", "tollgate")
	return &Tollgate{adapter: adapter, extractKey: keyFunc, logger: ctxLogger}
}

func (t *Tollgate) HTTPHandlerMiddleware(next http.Handler) http.Handler {
	return &tollgateHTTPHandler{
		next:   next,
		logger: t.logger,
		client: t,
	}
}

type tollgateHTTPHandler struct {
	next   http.Handler
	logger *slog.Logger
	client *Tollgate
}

// statusCapturingWriter wraps http.ResponseWriter to capture the status code
type statusCapturingWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (h *tollgateHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := h.client.extractKey(r)
	reserved, err := h.client.adapter.Reserve(r.Context(), key, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logger.Info("Reserved quota", "key", key, "reserved", reserved)

	if !reserved {
		http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
		return
	}

	// Wrap the ResponseWriter to capture the status code
	wrapper := &statusCapturingWriter{ResponseWriter: w, statusCode: http.StatusOK}
	h.next.ServeHTTP(wrapper, r)

	// Refund reserved quota if the request failed (status code >= 400)
	if wrapper.statusCode >= 400 {
		if _, err := h.client.adapter.Refund(r.Context(), key, 1); err != nil {
			// Log the refund error but don't fail the request
			// The request has already been processed
			_ = err // Acknowledge the error but continue
		}
		h.logger.Info("Refunded quota", "key", key, "refunded", 1)
	}
	// If successful (status < 400), keep the reserved quota (do nothing)
}
