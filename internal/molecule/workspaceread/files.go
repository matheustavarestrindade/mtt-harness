// Package workspaceread provides bounded, rooted, read-only project retrieval.
package workspaceread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const maximumEntries = 4096
const maximumFiles = 256
const maximumReadBytes = 2 * 1024 * 1024
const maximumFileBytes = 64 * 1024

type fileCandidate struct {
	reference harness.WorkspaceFileReference
	score     int
}

// Search examines at most 4096 entries, 256 files and 2 MiB of text. Individual
// files are limited to 64 KiB. Source bounds do not imply a complete repository scan.
func Search(operationContext context.Context, directory string, query harness.WorkspaceFileQuery) (harness.WorkspaceFileResult, error) {
	result := harness.WorkspaceFileResult{}
	if query.Limit == 0 {
		query.Limit = 5
	}
	if query.MaxBytes == 0 {
		query.MaxBytes = 8192
	}
	if query.Limit < 1 || query.Limit > 12 || query.MaxBytes < 256 || query.MaxBytes > 16384 {
		return result, fmt.Errorf("workspace file retrieval limits are invalid")
	}
	if strings.TrimSpace(query.Query) == "" || len(query.Query) > 8192 || !utf8.ValidString(query.Query) {
		return result, fmt.Errorf("workspace file query is empty, invalid UTF-8, or exceeds 8192 bytes")
	}
	root, operationError := os.OpenRoot(directory)
	if operationError != nil {
		return result, operationError
	}
	defer root.Close()
	terms := queryTerms(query.Query)
	directories := []string{"."}
	candidates := []fileCandidate{}
	for len(directories) > 0 {
		if operationError := operationContext.Err(); operationError != nil {
			return result, operationError
		}
		path := directories[0]
		directories = directories[1:]
		folder, operationError := openEntry(root, path)
		if operationError != nil {
			continue
		}
		for {
			entries, readError := folder.ReadDir(64)
			for _, entry := range entries {
				if result.EntriesVisited >= maximumEntries {
					result.Truncated = true
					break
				}
				result.EntriesVisited++
				if operationError := operationContext.Err(); operationError != nil {
					folder.Close()
					return result, operationError
				}
				if blockedName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				entryPath := filepath.Join(path, entry.Name())
				if entry.IsDir() {
					directories = append(directories, entryPath)
					continue
				}
				if !entry.Type().IsRegular() || !textExtension(entry.Name()) {
					continue
				}
				if result.FilesExamined >= maximumFiles || result.BytesExamined >= maximumReadBytes {
					result.Truncated = true
					break
				}
				result.FilesExamined++
				text, version, examined, readError := readTextFile(root, entryPath, maximumReadBytes-result.BytesExamined)
				result.BytesExamined += examined
				if readError != nil {
					continue
				}
				reference, score := rankExcerpt(filepath.ToSlash(entryPath), text, version, terms)
				if score == 0 {
					continue
				}
				candidates = append(candidates, fileCandidate{reference, score})
				sort.Slice(candidates, func(first, second int) bool {
					if candidates[first].score != candidates[second].score {
						return candidates[first].score > candidates[second].score
					}
					return candidates[first].reference.Path < candidates[second].reference.Path
				})
				if len(candidates) > query.Limit {
					candidates = candidates[:query.Limit]
				}
			}
			if result.Truncated || readError != nil {
				break
			}
		}
		folder.Close()
		if result.Truncated {
			break
		}
	}
	remaining := query.MaxBytes
	for _, candidate := range candidates {
		if len(candidate.reference.Text) > remaining {
			result.Truncated = true
			continue
		}
		result.References = append(result.References, candidate.reference)
		remaining -= len(candidate.reference.Text)
	}
	return result, operationContext.Err()
}

// Verify rejects changed, missing or newly excluded sources. It reads at most
// the same bounded regular-file snapshots as Search.
func Verify(operationContext context.Context, directory string, references []harness.WorkspaceFileReference) (bool, error) {
	if len(references) > 12 {
		return false, fmt.Errorf("workspace file verification exceeds 12 references")
	}
	root, operationError := os.OpenRoot(directory)
	if operationError != nil {
		return false, operationError
	}
	defer root.Close()
	for _, reference := range references {
		if operationError := operationContext.Err(); operationError != nil {
			return false, operationError
		}
		path := filepath.Clean(filepath.FromSlash(reference.Path))
		if !filepath.IsLocal(path) {
			return false, nil
		}
		for _, component := range strings.Split(filepath.ToSlash(path), "/") {
			if blockedName(component) {
				return false, nil
			}
		}
		_, version, _, operationError := readTextFile(root, path, maximumFileBytes)
		if operationError != nil || version != reference.Version {
			return false, nil
		}
	}
	return true, nil
}

func readTextFile(root *os.Root, path string, budget int) (string, string, int, error) {
	file, operationError := openEntry(root, path)
	if operationError != nil {
		return "", "", 0, operationError
	}
	defer file.Close()
	metadata, operationError := file.Stat()
	if operationError != nil {
		return "", "", 0, operationError
	}
	if !metadata.Mode().IsRegular() || metadata.Size() > maximumFileBytes || metadata.Size() > int64(budget) {
		return "", "", 0, fmt.Errorf("file is not a bounded regular file")
	}
	data, operationError := io.ReadAll(io.LimitReader(file, int64(min(maximumFileBytes, budget))+1))
	if operationError != nil {
		return "", "", len(data), operationError
	}
	if len(data) > maximumFileBytes || len(data) > budget || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", "", len(data), fmt.Errorf("file is not bounded UTF-8 text")
	}
	digest := sha256.Sum256(data)
	return string(data), hex.EncodeToString(digest[:]), len(data), nil
}

func blockedName(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, ".") || strings.HasPrefix(lower, "id_rsa") || strings.HasPrefix(lower, "id_ed25519") {
		return true
	}
	for _, suffix := range []string{".pem", ".key", ".p12", ".pfx", ".keystore", ".crt"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	switch lower {
	case "node_modules", "vendor", "dist", "build", "target", "coverage", "secrets", "credentials", "auth.json", "mtt.json", "mcp.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.sum":
		return true
	}
	return false
}

func textExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case "", ".go", ".rs", ".py", ".js", ".ts", ".tsx", ".jsx", ".svelte", ".vue", ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".sql", ".sh", ".css", ".html", ".java", ".kt", ".c", ".h", ".cpp", ".cs", ".rb", ".php":
		return true
	}
	return false
}

func queryTerms(text string) []string {
	stop := " the and for this that with from into doing current task update using use fix add work checking reading inspect implement implementing need should "
	seen := map[string]bool{}
	result := []string{}
	for _, term := range strings.FieldsFunc(strings.ToLower(text), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_'
	}) {
		if len(term) < 3 || seen[term] || strings.Contains(stop, " "+term+" ") {
			continue
		}
		seen[term] = true
		result = append(result, term)
		if len(result) == 24 {
			break
		}
	}
	return result
}

func rankExcerpt(path, text, version string, terms []string) (harness.WorkspaceFileReference, int) {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	bestLine, bestScore, pathScore := 0, 0, 0
	for _, term := range terms {
		if strings.Contains(strings.ToLower(path), term) {
			pathScore += 4
		}
	}
	if strings.EqualFold(filepath.Base(path), "AGENTS.md") {
		pathScore += 3
	}
	for index, line := range lines {
		score := 0
		for _, term := range terms {
			if strings.Contains(strings.ToLower(line), term) {
				score++
			}
		}
		if score > bestScore {
			bestLine, bestScore = index, score
		}
	}
	start, end := max(0, bestLine-3), min(len(lines), bestLine+9)
	excerpt := strings.Join(lines[start:end], "\n")
	for len(excerpt) > 4096 && end > start+1 {
		end--
		excerpt = strings.Join(lines[start:end], "\n")
	}
	if len(excerpt) > 4096 {
		return harness.WorkspaceFileReference{}, 0
	}
	return harness.WorkspaceFileReference{Path: path, StartLine: start + 1, EndLine: end, Text: excerpt, Version: version}, pathScore + bestScore
}
