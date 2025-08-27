package cache

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

// cachedHTTPHandler is a `http.Handler` that caches the responses.
type cachedHTTPHandler struct {
	next   http.Handler
	client *Cache
}

func (h *cachedHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := h.client
	next := h.next
	if c.cacheableMethod(r.Method) {
		sortURLParams(r.URL)
		key := generateKey(r.URL.String())
		if r.Method == http.MethodPost && r.Body != nil {
			body, err := io.ReadAll(r.Body)
			defer r.Body.Close()
			if err != nil {
				h.client.logger.Warn("Failed to read request body", "method", r.Method, "url", r.URL.String(), "error", err)
				next.ServeHTTP(w, r)
				return
			}
			reader := io.NopCloser(bytes.NewBuffer(body))
			key = generateKeyWithBody(r.URL.String(), body)
			r.Body = reader
		}

		params := r.URL.Query()
		if _, ok := params[c.refreshKey]; ok {
			delete(params, c.refreshKey)

			r.URL.RawQuery = params.Encode()
			key = generateKey(r.URL.String())

			h.client.logger.Info("Cache refresh requested", "key", key, "method", r.Method, "url", r.URL.String())
			c.adapter.Release(r.Context(), key)
		} else {
			b, ok := c.adapter.Get(r.Context(), key)
			if ok {
				response, err := BytesToResponse(b)
				if err != nil {
					h.client.logger.Warn("Failed to deserialize cached response", "key", key, "error", err)
					next.ServeHTTP(w, r)
					return
				}
				if response.Expiration.After(time.Now()) {
					response.LastAccess = time.Now()
					response.Frequency++
					c.adapter.Set(key, response.Bytes(), response.Expiration)

					h.client.logger.Info("Cache hit", "key", key, "method", r.Method, "url", r.URL.String(), "frequency", response.Frequency)
					//w.WriteHeader(http.StatusNotModified)
					for k, v := range response.Header {
						w.Header().Set(k, strings.Join(v, ","))
					}
					if c.writeExpiresHeader {
						w.Header().Set("Expires", response.Expiration.UTC().Format(http.TimeFormat))
					}
					if _, err := w.Write(response.Value); err != nil {
						// Log the error but continue - response was already committed
						// This error would be rare (client disconnect, etc.)
						_ = err // Acknowledge the error exists
					}
					return
				}

				h.client.logger.Info("Cache entry expired", "key", key, "expiration", response.Expiration)
				c.adapter.Release(r.Context(), key)
			}
		}

		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		statusCode := rw.statusCode
		value := rw.body
		now := time.Now()
		expires := now.Add(c.ttl)
		if statusCode < 400 {
			response := Response{
				Value:      value,
				Header:     rw.Header(),
				Expiration: expires,
				LastAccess: now,
				Frequency:  1,
			}
			c.adapter.Set(key, response.Bytes(), response.Expiration)
			h.client.logger.Info("Cache miss - new entry created", "key", key, "method", r.Method, "url", r.URL.String(), "status_code", statusCode, "expires", expires)
		} else {
			h.client.logger.Warn("Response not cached due to error status", "key", key, "method", r.Method, "url", r.URL.String(), "status_code", statusCode)
		}

		return
	}

	next.ServeHTTP(w, r)
}

// cacheRoundTripper is a `http.RoundTripper` that caches the responses.
type cacheRoundTripper struct {
	next   http.RoundTripper
	client *Cache
}

func (rt *cacheRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.client.cacheableMethod(r.Method) {
		sortURLParams(r.URL)
		key := generateKey(r.URL.String())
		if r.Method == http.MethodPost && r.Body != nil {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return rt.next.RoundTrip(r)
			}
			// Restore the body for downstream handlers
			r.Body = io.NopCloser(bytes.NewBuffer(body))
			key = generateKeyWithBody(r.URL.String(), body)
		}

		params := r.URL.Query()
		if _, ok := params[rt.client.refreshKey]; ok {
			delete(params, rt.client.refreshKey)

			r.URL.RawQuery = params.Encode()
			key = generateKey(r.URL.String())

			rt.client.adapter.Release(r.Context(), key)
		} else {
			b, ok := rt.client.adapter.Get(r.Context(), key)
			if ok {
				response, err := BytesToResponse(b)
				if err != nil {
					return rt.next.RoundTrip(r)
				}
				if response.Expiration.After(time.Now()) {
					response.LastAccess = time.Now()
					response.Frequency++
					rt.client.adapter.Set(key, response.Bytes(), response.Expiration)

					// Create a new response from the cached data
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBuffer(response.Value)),
						Header:     response.Header,
					}

					if rt.client.writeExpiresHeader {
						resp.Header.Set("Expires", response.Expiration.UTC().Format(http.TimeFormat))
					}
					return resp, nil
				}

				rt.client.adapter.Release(r.Context(), key)
			}
		}

		// Execute the original request
		resp, err := rt.next.RoundTrip(r)
		if err != nil {
			return nil, err
		}

		// Cache the response if successful
		if resp.StatusCode < 400 {
			// Read the body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				resp.Body.Close()
				return resp, nil
			}

			// Restore the body for downstream handlers
			resp.Body = io.NopCloser(bytes.NewBuffer(body))

			now := time.Now()
			expires := now.Add(rt.client.ttl)

			response := Response{
				Value:      body,
				Header:     resp.Header,
				Expiration: expires,
				LastAccess: now,
				Frequency:  1,
			}
			rt.client.adapter.Set(key, response.Bytes(), response.Expiration)
		}

		return resp, nil
	}

	return rt.next.RoundTrip(r)
}
