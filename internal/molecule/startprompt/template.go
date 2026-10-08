// Package startprompt loads and renders the harness-owned system instructions.
// Templates contain text substitutions, never executable expressions.
package startprompt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_.:-]*)\}\}|\{([A-Za-z_][A-Za-z0-9_.:-]*)\}`)

type templatePart struct {
	text     string
	variable string
}

// Template is immutable after parsing and can be rendered by concurrent sessions.
type Template struct {
	parts       []templatePart
	fingerprint [32]byte
}

// Fingerprint identifies the original, already-loaded template bytes.
func (template *Template) Fingerprint() string { return hex.EncodeToString(template.fingerprint[:]) }

// Load reads the file once. Relative paths use the process working directory.
// An empty file is valid and supplies no startup instructions.
func Load(path string) (*Template, error) {
	content, operationError := os.ReadFile(path)
	if operationError != nil {
		return nil, fmt.Errorf("read start prompt %q: %w", path, operationError)
	}
	template, operationError := Parse(string(content))
	if operationError != nil {
		return nil, fmt.Errorf("parse start prompt %q: %w", path, operationError)
	}
	return template, nil
}

// Parse recognizes {variable} and escaped {{variable}}. Other braces, including
// ordinary JSON objects, are literal. Tool names are resolved at request time.
func Parse(text string) (*Template, error) {
	template := &Template{fingerprint: sha256.Sum256([]byte(text))}
	previousEnd := 0
	for _, match := range placeholderPattern.FindAllStringSubmatchIndex(text, -1) {
		template.parts = append(template.parts, templatePart{text: text[previousEnd:match[0]]})
		previousEnd = match[1]
		if match[2] >= 0 {
			template.parts = append(template.parts, templatePart{text: "{" + text[match[2]:match[3]] + "}"})
			continue
		}
		variable := text[match[4]:match[5]]
		if !supportedVariable(variable) {
			return nil, fmt.Errorf("unknown variable {%s}; use {{%s}} for literal text", variable, variable)
		}
		template.parts = append(template.parts, templatePart{variable: variable})
	}
	template.parts = append(template.parts, templatePart{text: text[previousEnd:]})
	return template, nil
}

func supportedVariable(variable string) bool {
	switch variable {
	case "tool_list", "workspace", "session_id", "instance_id", "model", "agent_depth", "os", "arch":
		return true
	default:
		return strings.HasSuffix(variable, "_info") && variable != "_info"
	}
}

// Render substitutes values once, so braces in tool descriptions or workspace
// paths cannot trigger more substitutions. Repeated variables share one value
// within a request even if registry updates occur during rendering.
func (template *Template) Render(operationContext context.Context, values Values) (string, error) {
	var rendered strings.Builder
	expansions := map[string]string{}
	for _, part := range template.parts {
		if operationError := operationContext.Err(); operationError != nil {
			return "", operationError
		}
		if part.variable == "" {
			rendered.WriteString(part.text)
			continue
		}
		expansion, found := expansions[part.variable]
		if !found {
			var operationError error
			expansion, operationError = renderVariable(part.variable, values)
			if operationError != nil {
				return "", fmt.Errorf("render start prompt {%s}: %w", part.variable, operationError)
			}
			expansions[part.variable] = expansion
		}
		rendered.WriteString(expansion)
	}
	return rendered.String(), operationContext.Err()
}
