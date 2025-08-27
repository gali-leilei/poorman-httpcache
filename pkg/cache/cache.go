/*
MIT License

Copyright (c) 2018 Victor Springer

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package cache provides a cache middleware for HTTP requests.
package cache

import (
	"bytes"
	"encoding/gob"
	"hash/fnv"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Response is the cached response data structure.
type Response struct {
	// Value is the cached response value.
	Value []byte

	// Header is the cached response header.
	Header http.Header

	// Expiration is the cached response expiration date.
	Expiration time.Time

	// LastAccess is the last date a cached response was accessed.
	// Used by LRU and MRU algorithms.
	LastAccess time.Time

	// Frequency is the count of times a cached response is accessed.
	// Used for LFU and MFU algorithms.
	Frequency int
}

// Cache data structure for HTTP cache middleware.
type Cache struct {
	adapter            Adapter
	ttl                time.Duration
	refreshKey         string
	methods            []string
	writeExpiresHeader bool
	logger             *slog.Logger
}

// HTTPHandlerMiddleware is the HTTP cache middleware handler.
func (c *Cache) HTTPHandlerMiddleware(next http.Handler) http.Handler {
	return &cachedHTTPHandler{
		next:   next,
		client: c,
	}
}

// RoundTripperMiddleware is the HTTP cache middleware for RoundTripper.
func (c *Cache) RoundTripperMiddleware(next http.RoundTripper) http.RoundTripper {
	return &cacheRoundTripper{
		next:   next,
		client: c,
	}
}

func (c *Cache) cacheableMethod(method string) bool {
	for _, m := range c.methods {
		if method == m {
			return true
		}
	}
	return false
}

// BytesToResponse converts bytes array into Response data structure.
func BytesToResponse(b []byte) (Response, error) {
	var r Response
	dec := gob.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&r); err != nil {
		return Response{}, err
	}

	return r, nil
}

// Bytes converts Response data structure into bytes array.
func (r Response) Bytes() []byte {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	if err := enc.Encode(&r); err != nil {
		// This is unlikely to fail for Response struct, but if it does,
		// return empty bytes to prevent undefined behavior
		return []byte{}
	}

	return b.Bytes()
}

func sortURLParams(URL *url.URL) {
	params := URL.Query()
	for _, param := range params {
		sort.Slice(param, func(i, j int) bool {
			return param[i] < param[j]
		})
	}
	URL.RawQuery = params.Encode()
}

// KeyAsString can be used by adapters to convert the cache key from uint64 to string.
func KeyAsString(key uint64) string {
	return strconv.FormatUint(key, 36)
}

func generateKey(URL string) uint64 {
	hash := fnv.New64a()
	hash.Write([]byte(URL))

	return hash.Sum64()
}

func generateKeyWithBody(URL string, body []byte) uint64 {
	hash := fnv.New64a()
	body = append([]byte(URL), body...)
	hash.Write(body)

	return hash.Sum64()
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body = b
	return w.ResponseWriter.Write(b)
}
