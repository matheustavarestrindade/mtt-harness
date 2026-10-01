package toolsearch

import (
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Document struct {
	ID   string
	Text string
	// Summary is authoritative name/category/description text, also present in
	// Text. Lexical ranking gives it more weight than examples and schema prose.
	Summary string
}

// Describe builds an embedding-only document from authoritative tool metadata.
// Extra usage examples never replace model instructions or external MCP schemas.
func Describe(tool harness.Tool) Document {
	var document strings.Builder
	document.WriteString("Tool: " + tool.Name() + "\nCategories: " + strings.Join(tool.Categories(), ", "))
	document.WriteString("\nPurpose and behavior:\n" + tool.Description())
	if documentation, available := tool.(harness.ToolSearchDocumentation); available {
		document.WriteString("\nUsage examples:\n" + documentation.SearchDocument())
	}
	document.WriteString("\nInput specification (JSON Schema):\n" + string(tool.InputSchema().JSON))
	return Document{ID: tool.Name(), Text: document.String(), Summary: tool.Name() + "\n" + strings.Join(tool.Categories(), ", ") + "\n" + tool.Description()}
}
