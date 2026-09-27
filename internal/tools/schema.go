package tools

import (
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func schema(raw string) atom.Schema {
	return atom.Schema{JSON: json.RawMessage(raw)}
}
