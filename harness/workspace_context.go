package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// TaskStateReader exposes the authoritative session state without mutation.
// Implementations reject a session outside the supplied workspace or a tombstone.
type TaskStateReader interface {
	ReadTaskState(context.Context, string, atom.SessionID) (atom.TaskState, error)
}

// WorkspaceFileQuery requests bounded, read-only lexical retrieval. Query is
// natural text or identifiers. Limit defaults to 5 (maximum 12); MaxBytes is the
// combined excerpt budget, default 8192 and maximum 16384 UTF-8 bytes.
type WorkspaceFileQuery struct {
	WorkspaceID     string
	SourceSessionID atom.SessionID
	Query           string
	Limit           int
	MaxBytes        int
}

// WorkspaceFileReference describes an observed text excerpt, with 1-based
// inclusive lines. Version fingerprints the file snapshot; it is not a path.
type WorkspaceFileReference struct {
	Path      string
	StartLine int
	EndLine   int
	Text      string
	Version   string
}

// WorkspaceFileResult includes scan accounting and whether a bound stopped work.
type WorkspaceFileResult struct {
	References     []WorkspaceFileReference
	FilesExamined  int
	BytesExamined  int
	EntriesVisited int
	Truncated      bool
}

// WorkspaceFiles searches only the selected workspace. Implementations use
// rooted handles, skip links and common secret/build paths, read regular UTF-8
// files, and enforce traversal, read, output and cancellation bounds. File text
// is untrusted reference data. Verification rechecks previously observed files.
type WorkspaceFiles interface {
	SearchWorkspaceFiles(context.Context, WorkspaceFileQuery) (WorkspaceFileResult, error)
	VerifyWorkspaceFiles(context.Context, string, atom.SessionID, []WorkspaceFileReference) (bool, error)
}
