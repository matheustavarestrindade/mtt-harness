package tools

import (
	"container/heap"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Retained matches own observed metadata, not a live directory handle. Default
// output needs no per-file stat. Names are relative to the requested glob root.
type globDirectoryEntry struct {
	path        string
	mode        os.FileMode
	information os.FileInfo
}

func (entry globDirectoryEntry) Name() string      { return entry.path }
func (entry globDirectoryEntry) IsDir() bool       { return entry.mode.IsDir() }
func (entry globDirectoryEntry) Type() os.FileMode { return entry.mode }
func (entry globDirectoryEntry) Info() (os.FileInfo, error) {
	if entry.information == nil {
		return nil, os.ErrNotExist
	}
	return entry.information, nil
}

func collectGlobPage(operationContext context.Context, path string, options fileGlobOptions, cursor string, limit int, fields []string) ([]os.DirEntry, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	after, operationError := globCursorPosition(path, options, cursor)
	if operationError != nil {
		return nil, operationError
	}
	root, operationError := os.OpenRoot(path)
	if operationError != nil {
		return nil, operationError
	}
	defer root.Close()
	searchRoot := root
	defer func() {
		if searchRoot != root {
			_ = searchRoot.Close()
		}
	}()
	base, _ := doublestar.SplitPattern(options.Pattern)
	// Escapes and alternatives remain the matcher's responsibility. A plain
	// literal prefix can narrow I/O without changing the pattern's meaning.
	if strings.ContainsAny(base, `\*?[{`) {
		base = "."
	}
	prefix := ""
	if base != "." && base != "" {
		for _, name := range strings.Split(base, "/") {
			if prefix != "" {
				prefix += "/"
			}
			prefix += name
			if options.matchesGlobExclusion(prefix, true) {
				return nil, nil
			}
			child, operationError := openGlobDirectory(operationContext, searchRoot, name)
			if os.IsNotExist(operationError) {
				return nil, nil
			}
			if operationError != nil {
				return nil, operationError
			}
			if child == nil {
				return nil, nil
			}
			if searchRoot != root {
				if operationError := searchRoot.Close(); operationError != nil {
					_ = child.Close()
					return nil, operationError
				}
			}
			searchRoot = child
		}
	}
	entries := directoryEntryHeap{}
	if prefix != "" && globKindMatches(options.Kind, os.ModeDir) && prefix > after && (doublestar.MatchUnvalidated(options.Pattern, prefix) || doublestar.MatchUnvalidated(options.Pattern, prefix+"/")) {
		retained := globDirectoryEntry{path: prefix, mode: os.ModeDir}
		if len(fields) > 0 {
			retained.information, operationError = searchRoot.Stat(".")
			if operationError != nil {
				return nil, operationError
			}
		}
		heap.Push(&entries, retained)
	}
	operationError = walkGlobDirectory(operationContext, searchRoot, prefix, globPatternDepth(options.Pattern), func(parent *os.Root, relative string, entry os.DirEntry) (bool, error) {
		if options.matchesGlobExclusion(relative, entry.IsDir()) {
			return false, nil
		}
		matches := doublestar.MatchUnvalidated(options.Pattern, relative)
		if entry.IsDir() && !matches {
			matches = doublestar.MatchUnvalidated(options.Pattern, relative+"/")
		}
		if !matches || relative <= after || !globKindMatches(options.Kind, entry.Type()) {
			return true, nil
		}
		if len(entries) == limit+1 && relative >= entries[0].Name() {
			return true, nil
		}
		retained := globDirectoryEntry{path: relative, mode: entry.Type()}
		if len(fields) > 0 {
			information, operationError := parent.Lstat(entry.Name())
			if operationError != nil && !os.IsNotExist(operationError) {
				return false, operationError
			}
			retained.information = information
		}
		if len(entries) < limit+1 {
			heap.Push(&entries, retained)
		} else {
			entries[0] = retained
			heap.Fix(&entries, 0)
		}
		return true, nil
	})
	if operationError != nil {
		return nil, operationError
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int { return strings.Compare(left.Name(), right.Name()) })
	return entries, nil
}

func globKindMatches(kind string, mode os.FileMode) bool {
	switch kind {
	case "", "file":
		return mode.IsRegular()
	case "directory":
		return mode.IsDir()
	case "link":
		return mode&os.ModeSymlink != 0
	case "all":
		return true
	default:
		return false
	}
}

func (results *fileActionResults) glob(operationContext context.Context, path string, options fileGlobOptions, requestedLimit *int, cursor string, fields []string) (string, error) {
	limit := directoryPreviewDefaultEntries
	if requestedLimit != nil {
		limit = *requestedLimit
	}
	limit = min(limit, results.budget.lines)
	entries, operationError := collectGlobPage(operationContext, path, options, cursor, limit, fields)
	if operationError != nil {
		return "", operationError
	}
	var output strings.Builder
	returned := 0
	for _, entry := range entries {
		if operationError := operationContext.Err(); operationError != nil {
			return "", operationError
		}
		if returned == limit {
			break
		}
		row, operationError := formatDirectoryEntry(entry, fields)
		if operationError != nil {
			return "", operationError
		}
		if len(row) > filePreviewMaxBytes {
			return "", fmt.Errorf("glob result exceeds the %d-byte preview limit", filePreviewMaxBytes)
		}
		if output.Len()+len(row) > results.budget.bytes {
			break
		}
		output.WriteString(row)
		cursor = encodeGlobCursor(path, options, entry.Name())
		returned++
	}
	results.consume(output.String())
	if returned < len(entries) {
		return output.String() + fmt.Sprintf("\n[Glob page limited to %d entries and the shared 200-line/16-KiB budget. Continuation for this root and query: {\"cursor\":%q}.]\n", limit, cursor), nil
	}
	if returned == 0 {
		return "(no matches after this cursor)", nil
	}
	return output.String(), nil
}
