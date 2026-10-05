package tools

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type filePreviewLimits struct {
	lines int
	bytes int
}

type fileActionResults struct {
	parts  []string
	budget filePreviewLimits
}

func newFileActionResults() *fileActionResults {
	return &fileActionResults{budget: filePreviewLimits{lines: filePreviewMaxLines, bytes: filePreviewMaxBytes}}
}

func (results *fileActionResults) consume(text string) {
	results.budget.lines = max(0, results.budget.lines-countFileLines([]byte(text)))
	results.budget.bytes = max(0, results.budget.bytes-len(text))
}

func (results *fileActionResults) read(operationContext context.Context, source io.Reader, selection fileTextSelection, lineNumbers bool) (string, error) {
	preview, operationError := readFileTextPreviewWithin(operationContext, source, selection, lineNumbers, results.budget)
	if operationError != nil {
		return "", operationError
	}
	results.consume(preview.content)
	if preview.nextLine == 0 {
		return preview.content, nil
	}
	continuation := fmt.Sprintf(`{"start_line":%d`, preview.nextLine)
	if preview.nextByte > 0 {
		continuation += fmt.Sprintf(`,"start_byte":%d`, preview.nextByte)
	}
	if selection.EndLine != nil {
		continuation += fmt.Sprintf(`,"end_line":%d`, *selection.EndLine)
	}
	continuation += "}"
	return preview.content + "\n\n[Shared file_actions preview budget reached: 200 lines or 16 KiB including labels. Continuation for this path: " + continuation + ". This notice is not file content.]", nil
}

func (results *fileActionResults) add(label, text string) {
	if text == "" {
		text = "(empty output)"
	}
	results.parts = append(results.parts, label+"\n"+text)
}

func (results *fileActionResults) text() string { return strings.Join(results.parts, "\n\n") }
