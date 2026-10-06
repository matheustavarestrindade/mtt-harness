package tools

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

const globPatternMaximumCharacters = 1024
const globCursorMaximumBytes = 32768

type fileGlobOptions struct {
	Pattern string   `json:"pattern,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	Kind    string   `json:"kind,omitempty"`
}

func (options fileGlobOptions) validateGlobOptions(limit *int, cursor string, fields []string) error {
	if operationError := validateDirectoryPreview(limit, "", fields); operationError != nil {
		return operationError
	}
	if operationError := validateGlobPattern(options.Pattern); operationError != nil {
		return operationError
	}
	if len(options.Exclude) > 16 {
		return fmt.Errorf("exclude has more than 16 patterns")
	}
	for _, pattern := range options.Exclude {
		if operationError := validateGlobPattern(pattern); operationError != nil {
			return fmt.Errorf("exclude: %w", operationError)
		}
	}
	if !slices.Contains([]string{"", "file", "directory", "link", "all"}, options.Kind) {
		return fmt.Errorf("glob kind must be file, directory, link, or all")
	}
	_, _, operationError := decodeGlobCursor(cursor)
	return operationError
}

func validateGlobPattern(pattern string) error {
	if pattern == "" || utf8.RuneCountInString(pattern) > globPatternMaximumCharacters {
		return fmt.Errorf("glob pattern must contain 1-%d characters", globPatternMaximumCharacters)
	}
	if !isRelativeGlobPath(pattern) {
		return fmt.Errorf("glob pattern must be relative to its directory without parent traversal")
	}
	if !doublestar.ValidatePattern(pattern) {
		return fmt.Errorf("glob pattern is invalid: %w", doublestar.ErrBadPattern)
	}
	return nil
}

func isRelativeGlobPath(path string) bool {
	if strings.HasPrefix(path, "/") || filepath.IsAbs(path) || filepath.VolumeName(path) != "" || strings.ContainsRune(path, 0) {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." || segment == "." {
			return false
		}
	}
	return true
}

func (options fileGlobOptions) matchesGlobExclusion(path string, directory bool) bool {
	for _, pattern := range options.Exclude {
		if doublestar.MatchUnvalidated(pattern, path) || directory && doublestar.MatchUnvalidated(pattern, path+"/") {
			return true
		}
	}
	return false
}

func globPatternDepth(pattern string) int {
	if strings.Contains(pattern, "**") {
		return -1
	}
	return strings.Count(strings.TrimSuffix(pattern, "/"), "/") + 1
}
