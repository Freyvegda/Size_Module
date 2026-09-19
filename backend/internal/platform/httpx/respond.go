// Package httpx holds small HTTP helpers shared by the router and the modules.
// It exists as its own package so modules never need to import the router.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON writes v as a JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writing response", "error", err)
	}
}

// Error writes a structured error response.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// DecodeJSON reads a JSON request body with a size limit and rejects unknown
// fields, so typos surface immediately instead of being silently dropped.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 4 << 20 // 4 MiB
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
