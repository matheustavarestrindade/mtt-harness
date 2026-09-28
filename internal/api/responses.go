package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (server *Server) health(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(responseWriter http.ResponseWriter, status int, value any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(status)
	_ = json.NewEncoder(responseWriter).Encode(value)
}

func writeError(responseWriter http.ResponseWriter, status int, message string) {
	writeJSON(responseWriter, status, map[string]string{"error": message})
}

// respondToError writes a failed operation once and reports whether the handler
// must return. Successful operations leave the response untouched.
func respondToError(responseWriter http.ResponseWriter, status int, operationError error) bool {
	if operationError == nil {
		return false
	}
	writeError(responseWriter, status, operationError.Error())
	return true
}

func readJSON(request *http.Request, value any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 16<<20))
	if operationError := decoder.Decode(value); operationError != nil {
		return operationError
	}
	var extra any
	if operationError := decoder.Decode(&extra); operationError != io.EOF {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}
