package tools

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"slices"
)

func globQueryFingerprint(root string, options fileGlobOptions) []byte {
	exclusions := append([]string{}, options.Exclude...)
	slices.Sort(exclusions)
	kind := options.Kind
	if kind == "" {
		kind = "file"
	}
	fingerprint := sha256.Sum256([]byte(fmt.Sprintf("%q\n%q\n%q\n%q", root, options.Pattern, exclusions, kind)))
	return fingerprint[:]
}

func encodeGlobCursor(root string, options fileGlobOptions, after string) string {
	value := append([]byte{1}, globQueryFingerprint(root, options)...)
	value = append(value, []byte(after)...)
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeGlobCursor(cursor string) ([]byte, string, error) {
	if cursor == "" {
		return nil, "", nil
	}
	if len(cursor) > globCursorMaximumBytes {
		return nil, "", fmt.Errorf("glob cursor exceeds %d bytes", globCursorMaximumBytes)
	}
	value, operationError := base64.RawURLEncoding.DecodeString(cursor)
	if operationError != nil {
		return nil, "", fmt.Errorf("invalid glob cursor: %w", operationError)
	}
	if len(value) < 34 || value[0] != 1 {
		return nil, "", fmt.Errorf("invalid glob cursor format")
	}
	after := string(value[33:])
	if !isRelativeGlobPath(after) {
		return nil, "", fmt.Errorf("invalid glob cursor path")
	}
	return value[1:33], after, nil
}

func globCursorPosition(root string, options fileGlobOptions, cursor string) (string, error) {
	fingerprint, after, operationError := decodeGlobCursor(cursor)
	if operationError != nil {
		return "", operationError
	}
	if cursor != "" && !bytes.Equal(fingerprint, globQueryFingerprint(root, options)) {
		return "", fmt.Errorf("glob cursor does not match the root or query")
	}
	return after, nil
}
