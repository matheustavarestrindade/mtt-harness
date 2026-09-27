package api

import "net/http"

type WebSocket struct{}

func (WebSocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "websocket: not implemented", http.StatusNotImplemented)
}
