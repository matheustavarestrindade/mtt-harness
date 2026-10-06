package tools

import (
	"container/heap"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// A max heap keeps only the next bounded page while scanning in directory order.
// No complete directory-name list or recursively resolved symlink tree is held.
type directoryEntryHeap []os.DirEntry

func (entries directoryEntryHeap) Len() int { return len(entries) }
func (entries directoryEntryHeap) Less(left, right int) bool {
	return entries[left].Name() > entries[right].Name()
}
func (entries directoryEntryHeap) Swap(left, right int) {
	entries[left], entries[right] = entries[right], entries[left]
}
func (entries *directoryEntryHeap) Push(value any) { *entries = append(*entries, value.(os.DirEntry)) }
func (entries *directoryEntryHeap) Pop() any {
	last := (*entries)[len(*entries)-1]
	*entries = (*entries)[:len(*entries)-1]
	return last
}

func collectDirectoryPage(operationContext context.Context, path, cursor string, limit int) ([]os.DirEntry, error) {
	afterName, operationError := base64.RawURLEncoding.DecodeString(cursor)
	if operationError != nil {
		return nil, operationError
	}
	information, operationError := os.Stat(path)
	if operationError != nil {
		return nil, operationError
	}
	if !information.IsDir() {
		return nil, fmt.Errorf("list requires a directory")
	}
	directory, operationError := os.Open(path)
	if operationError != nil {
		return nil, operationError
	}
	defer directory.Close()
	information, operationError = directory.Stat()
	if operationError != nil {
		return nil, operationError
	}
	if !information.IsDir() {
		return nil, fmt.Errorf("list requires a directory")
	}
	entries := directoryEntryHeap{}
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		batch, readError := directory.ReadDir(128)
		for _, entry := range batch {
			if entry.Name() <= string(afterName) {
				continue
			}
			if len(entries) < limit+1 {
				heap.Push(&entries, entry)
				continue
			}
			if entry.Name() < entries[0].Name() {
				entries[0] = entry
				heap.Fix(&entries, 0)
			}
		}
		if errors.Is(readError, io.EOF) {
			break
		}
		if readError != nil {
			return nil, readError
		}
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int { return strings.Compare(left.Name(), right.Name()) })
	return entries, nil
}

func (results *fileActionResults) list(operationContext context.Context, path string, requestedLimit *int, cursor string, fields []string) (string, error) {
	limit := directoryPreviewDefaultEntries
	if requestedLimit != nil {
		limit = *requestedLimit
	}
	limit = min(limit, results.budget.lines)
	entries, operationError := collectDirectoryPage(operationContext, path, cursor, limit)
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
		if output.Len()+len(row) > results.budget.bytes {
			break
		}
		output.WriteString(row)
		cursor = base64.RawURLEncoding.EncodeToString([]byte(entry.Name()))
		returned++
	}
	results.consume(output.String())
	if returned < len(entries) {
		return output.String() + fmt.Sprintf("\n[Directory page limited to %d entries and the shared 200-line/16-KiB budget. Continuation for this path: {\"cursor\":%q}.]\n", limit, cursor), nil
	}
	if returned == 0 {
		return "(no entries after this cursor)", nil
	}
	return output.String(), nil
}

func directoryEntryKind(mode os.FileMode) string {
	switch {
	case mode&os.ModeSymlink != 0:
		return "L"
	case mode.IsDir():
		return "D"
	case mode.IsRegular():
		return "F"
	default:
		return "S"
	}
}

// Default listings need only the name and directory-entry type. Additional stat
// data is obtained only for requested metadata and never follows a symlink.
func formatDirectoryEntry(entry os.DirEntry, fields []string) (string, error) {
	var row strings.Builder
	fmt.Fprintf(&row, "%s %q", directoryEntryKind(entry.Type()), entry.Name())
	if len(fields) == 0 {
		return row.String() + "\n", nil
	}
	information, operationError := entry.Info()
	if operationError != nil && !os.IsNotExist(operationError) {
		return "", operationError
	}
	for _, field := range fields {
		value := "?"
		if information != nil {
			switch field {
			case "size":
				value = fmt.Sprint(information.Size())
			case "permissions":
				mode := uint32(information.Mode().Perm())
				if information.Mode()&os.ModeSetuid != 0 {
					mode |= 04000
				}
				if information.Mode()&os.ModeSetgid != 0 {
					mode |= 02000
				}
				if information.Mode()&os.ModeSticky != 0 {
					mode |= 01000
				}
				value = fmt.Sprintf("%04o", mode)
			case "owner", "group":
				owner, group, available := fileOwnerIDs(information)
				if available {
					if field == "owner" {
						value = fmt.Sprint(owner)
					} else {
						value = fmt.Sprint(group)
					}
				}
			case "modified":
				value = information.ModTime().UTC().Format(time.RFC3339Nano)
			}
		}
		fmt.Fprintf(&row, "\t%s=%s", field, value)
	}
	return row.String() + "\n", nil
}
