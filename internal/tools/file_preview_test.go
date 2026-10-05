package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func splitPreviewContinuation(test *testing.T, text string) (string, map[string]any) {
	test.Helper()
	noticeStart := strings.LastIndex(text, "\n\n[File text preview truncated")
	if noticeStart < 0 {
		return text, nil
	}
	notice := text[noticeStart:]
	start, end := strings.IndexByte(notice, '{'), strings.IndexByte(notice, '}')
	if start < 0 || end < start {
		test.Fatalf("truncation has no usable cursor: %s", notice)
	}
	var continuation map[string]any
	testutil.RequireNoError(test, json.Unmarshal([]byte(notice[start:end+1]), &continuation))
	return text[:noticeStart], continuation
}

func TestReadContinuationReconstructsEveryFileByte(test *testing.T) {
	for _, scenario := range []struct{ name, content string }{
		{"many lines", strings.Repeat("line\r\n", 603) + "last"},
		{"long line", strings.Repeat("x", 70000) + "\nnext\n"},
		{"UTF-8 across caps", strings.Repeat("日本語😀", 7000) + "\r\nlast"},
		{"mixed", "first\n\n" + strings.Repeat("x", 16370) + "😀\r\n" + strings.Repeat("tail\n", 400)},
	} {
		for _, numbered := range []bool{false, true} {
			test.Run(fmt.Sprintf("%s/numbered=%t", scenario.name, numbered), func(test *testing.T) {
				workspace := test.TempDir()
				testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "source.txt"), []byte(scenario.content), 0o600))
				operationContext := harness.WithWorkspace(context.Background(), workspace)
				arguments := map[string]any{"path": "source.txt"}
				var reconstructed strings.Builder
				for page := 0; page < 100; page++ {
					result, operationError := NewRead(numbered).Run(operationContext, fileCall(test, arguments))
					testutil.RequireNoError(test, operationError)
					text, continuation := splitPreviewContinuation(test, result.Text())
					if len(text) > filePreviewMaxBytes || countFileLines([]byte(text)) > filePreviewMaxLines || !utf8.ValidString(text) {
						test.Fatalf("page exceeds limits or splits UTF-8: %d bytes", len(text))
					}
					if numbered {
						lineNumber := strings.Count(reconstructed.String(), "\n") + 1
						for _, line := range strings.SplitAfter(text, "\n") {
							if line == "" {
								continue
							}
							prefix := strconv.Itoa(lineNumber) + ": "
							if !strings.HasPrefix(line, prefix) {
								test.Fatalf("wrong absolute label: expected %q in %.60q", prefix, line)
							}
							reconstructed.WriteString(strings.TrimPrefix(line, prefix))
							lineNumber++
						}
					} else {
						reconstructed.WriteString(text)
					}
					if continuation == nil {
						break
					}
					if len(text) == 0 {
						test.Fatal("continuation made no progress")
					}
					continuation["path"] = "source.txt"
					arguments = continuation
				}
				if reconstructed.String() != scenario.content {
					test.Fatalf("pagination repeated, omitted or changed bytes: got %d, want %d", reconstructed.Len(), len(scenario.content))
				}
			})
		}
	}
}

func TestReadCapsRespectEOFAndRequestedEnd(test *testing.T) {
	for _, scenario := range []struct {
		name, content string
		endLine       *int
	}{
		{"exact line cap", strings.Repeat("x\n", 200), nil},
		{"exact byte cap", strings.Repeat("x", 16384), nil},
		{"exact byte cap with LF", strings.Repeat("x", 16383) + "\n", nil},
		{"bounded range", strings.Repeat("x\n", 250), new(200)},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			preview, operationError := readFileTextPreview(context.Background(), strings.NewReader(scenario.content), fileTextSelection{lineRange: lineRange{EndLine: scenario.endLine}}, false)
			testutil.RequireNoError(test, operationError)
			if preview.nextLine != 0 {
				test.Fatalf("complete selected output incorrectly truncated: %+v", preview)
			}
			expected := scenario.content
			if scenario.endLine != nil {
				expected = strings.Repeat("x\n", *scenario.endLine)
			}
			if preview.content != expected {
				test.Fatal("output changed at a preview boundary")
			}
		})
	}
	preview, operationError := readFileTextPreview(context.Background(), strings.NewReader(strings.Repeat("x", 20000)+"\nlater"), fileTextSelection{lineRange: lineRange{EndLine: new(1)}}, false)
	testutil.RequireNoError(test, operationError)
	_, continuation := splitPreviewContinuation(test, preview.render(new(1)))
	if continuation["end_line"] != float64(1) || continuation["start_byte"] != float64(16384) {
		test.Fatalf("continuation lost its range or byte position: %+v", continuation)
	}
}

type countingPreviewReader struct {
	io.Reader
	bytesRead int
}

func (reader *countingPreviewReader) Read(destination []byte) (int, error) {
	count, operationError := reader.Reader.Read(destination)
	reader.bytesRead += count
	return count, operationError
}

func TestReadStopsBeforeLoadingAnOversizedLine(test *testing.T) {
	reader := &countingPreviewReader{Reader: strings.NewReader(strings.Repeat("x", 1<<20))}
	preview, operationError := readFileTextPreview(context.Background(), reader, fileTextSelection{}, false)
	testutil.RequireNoError(test, operationError)
	if reader.bytesRead > 64*1024 || len(preview.content) != filePreviewMaxBytes || preview.nextByte != filePreviewMaxBytes {
		test.Fatalf("read consumed too much input or lost its continuation: read=%d, preview=%+v", reader.bytesRead, preview)
	}
}

func TestReadRejectsInvalidContinuationWithoutLosingErrorCauses(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("日本\r\nnext"), 0o600))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	for _, offset := range []int{-1, 1, 2, 8, 99} {
		result, operationError := NewRead(false).Run(operationContext, fileCall(test, map[string]any{"path": "source.txt", "start_byte": offset}))
		if operationError == nil || result.Status != atom.StatusError || result.Error != operationError.Error() {
			test.Fatalf("invalid byte offset %d did not fail clearly: %+v, %v", offset, result, operationError)
		}
	}
	cancelledContext, cancelOperation := context.WithCancel(operationContext)
	cancelOperation()
	_, operationError := NewRead(false).Run(cancelledContext, fileCall(test, map[string]any{"path": "source.txt"}))
	if !errors.Is(operationError, context.Canceled) {
		test.Fatalf("lost cancellation cause: %v", operationError)
	}
}
