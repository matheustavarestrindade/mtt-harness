package api

import (
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
)

func (server *Server) resolvePermission(responseWriter http.ResponseWriter, request *http.Request) {
	var input struct {
		Kind  atom.VerdictKind `json:"kind"`
		Scope atom.Scope       `json:"scope"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	decision := atom.PermissionDecision{
		Kind:      input.Kind,
		Scope:     input.Scope,
		CreatedAt: time.Now(),
	}
	if decision.Scope == "" {
		decision.Scope = atom.ScopeOnce
	}
	if !permission.ValidDecision(decision) {
		writeError(responseWriter, http.StatusBadRequest, "kind must be allow or deny; scope must be once, session, or always")
		return
	}
	if !server.broker.Resolve(request.PathValue("id"), decision) {
		writeError(responseWriter, http.StatusNotFound, "the permission request is not open")
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "resolved"})
}
