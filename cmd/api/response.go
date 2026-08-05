package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// APIError is the canonical JSON error envelope.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorEnvelope wraps APIError so the response shape is {"error": {...}}.
type errorEnvelope struct {
	Error APIError `json:"error"`
}

// writeError emits the standard error envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: APIError{Code: code, Message: message}})
}

// decodeJSON parses a JSON body with sane limits and a 4 MiB cap.
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("missing body")
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return errors.New("empty body")
	}
	return json.Unmarshal(body, dst)
}
