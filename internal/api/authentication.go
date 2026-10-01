package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (server *Server) authenticateAPIRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			next.ServeHTTP(responseWriter, request)
			return
		}
		expectedToken := server.token
		if server.store != nil {
			storedToken, operationError := server.store.Settings().Get(request.Context(), "", "api_token")
			if respondToError(responseWriter, http.StatusServiceUnavailable, operationError) {
				return
			}
			if storedToken != "" {
				expectedToken = storedToken
			}
		}
		if expectedToken == "" {
			next.ServeHTTP(responseWriter, request)
			return
		}
		header := request.Header.Get("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == header {
			token = request.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			writeError(responseWriter, http.StatusUnauthorized, "the token is not correct")
			return
		}
		next.ServeHTTP(responseWriter, request)
	})
}
