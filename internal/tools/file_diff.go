package tools

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
)

const (
	fileDiffContextLines    = 3
	fileDiffPreviewBytes    = 16 * 1024
	fileDiffPreviewLines    = 200
	fileDiffComparisonBytes = 256 * 1024
	fileDiffComparisonLines = 4000
)

func renderFileEditDiff(path string, original, updated []byte, existed bool) string {
	if !utf8.Valid(original) || !utf8.Valid(updated) || bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(updated, 0) >= 0 {
		return "Diff preview omitted: the before or after content contains non-text data. The full edit succeeded."
	}
	before, after, skippedLines := selectFileDiffWindow(original, updated)
	if len(before)+len(after) > fileDiffComparisonBytes || countFileLines(before)+countFileLines(after) > fileDiffComparisonLines {
		return fmt.Sprintf("Diff preview omitted: the changed region exceeds 256 KiB or 4000 combined lines. The full edit succeeded. Use read from line %d to inspect the updated file; an empty file has no readable lines.", skippedLines+1)
	}
	beforeLines, afterLines := splitFileDiffLines(before), splitFileDiffLines(after)
	preview := fileDiffPreview{}
	previousPath := strconv.Quote(path)
	if !existed {
		previousPath = "/dev/null"
	}
	preview.append(fmt.Sprintf("--- %s\n+++ %s\n", previousPath, strconv.Quote(path)))
	for _, group := range difflib.NewMatcher(beforeLines, afterLines).GetGroupedOpCodes(fileDiffContextLines) {
		first, last := group[0], group[len(group)-1]
		preview.append(fmt.Sprintf("@@ -%s +%s @@\n", formatFileDiffRange(skippedLines+first.I1, last.I2-first.I1), formatFileDiffRange(skippedLines+first.J1, last.J2-first.J1)))
		for _, operation := range group {
			if operation.Tag == 'e' {
				preview.appendSourceLines(" ", beforeLines[operation.I1:operation.I2])
				continue
			}
			if operation.Tag == 'r' || operation.Tag == 'd' {
				preview.appendSourceLines("-", beforeLines[operation.I1:operation.I2])
			}
			if operation.Tag == 'r' || operation.Tag == 'i' {
				preview.appendSourceLines("+", afterLines[operation.J1:operation.J2])
			}
		}
	}
	if preview.truncated {
		return preview.content.String() + "\nDiff preview truncated at 200 lines or 16 KiB. The full edit succeeded. Use read to inspect the remaining content."
	}
	return preview.content.String()
}

// Remove shared outer text before line matching, retaining context and absolute
// line offsets. A small edit in a large file must not diff the entire file.
func selectFileDiffWindow(original, updated []byte) ([]byte, []byte, int) {
	sharedPrefix := 0
	for sharedPrefix < len(original) && sharedPrefix < len(updated) && original[sharedPrefix] == updated[sharedPrefix] {
		sharedPrefix++
	}
	start := bytes.LastIndexByte(original[:sharedPrefix], '\n') + 1
	for range fileDiffContextLines {
		if start == 0 {
			break
		}
		start = bytes.LastIndexByte(original[:start-1], '\n') + 1
	}
	sharedSuffix := 0
	for sharedSuffix < len(original)-sharedPrefix && sharedSuffix < len(updated)-sharedPrefix && original[len(original)-sharedSuffix-1] == updated[len(updated)-sharedSuffix-1] {
		sharedSuffix++
	}
	beforeEnd, afterEnd := len(original)-sharedSuffix, len(updated)-sharedSuffix
	// Finish the line containing the last changed byte, then retain context.
	for range fileDiffContextLines + 1 {
		newline := bytes.IndexByte(original[beforeEnd:], '\n')
		if newline < 0 {
			beforeEnd, afterEnd = len(original), len(updated)
			break
		}
		beforeEnd += newline + 1
		afterEnd += newline + 1
	}
	return original[start:beforeEnd], updated[start:afterEnd], bytes.Count(original[:start], []byte{'\n'})
}

func splitFileDiffLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(content), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func formatFileDiffRange(start, length int) string {
	if length > 0 {
		start++
	}
	return fmt.Sprintf("%d,%d", start, length)
}

type fileDiffPreview struct {
	content   strings.Builder
	lines     int
	truncated bool
}

func (preview *fileDiffPreview) append(text string) {
	if preview.truncated {
		return
	}
	lines := strings.Count(text, "\n")
	if preview.content.Len()+len(text) > fileDiffPreviewBytes || preview.lines+lines > fileDiffPreviewLines {
		preview.truncated = true
		return
	}
	preview.content.WriteString(text)
	preview.lines += lines
}

func (preview *fileDiffPreview) appendSourceLines(prefix string, lines []string) {
	for _, line := range lines {
		if preview.truncated {
			return
		}
		text := prefix + line
		if !strings.HasSuffix(line, "\n") {
			text += "\n\\ No newline at end of file\n"
		}
		preview.append(text)
	}
}
