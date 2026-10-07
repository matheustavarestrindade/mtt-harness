package embedding

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestContextEmbeddingConfigurationUsesItsOwnSection(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "providers.json")
	testutil.RequireNoError(t, os.WriteFile(path, []byte(`{"tool_search":{"model_directory":"tool-model"},"context_embeddings":{"backend":"embeddinggemma2","model_directory":"memory-model","model_file":"onnx/model_quantized.onnx","runtime_library":"native/runtime.so"},"providers":[]}`), 0600))
	configuration, operationError := LoadContextConfiguration(path)
	testutil.RequireNoError(t, operationError)
	if configuration.ModelDirectory != filepath.Join(directory, "memory-model") || configuration.RuntimeLibrary != filepath.Join(directory, "native/runtime.so") || configuration.ChunkTokens != 1024 || configuration.Threads != 2 {
		t.Fatalf("unexpected context settings: %+v", configuration)
	}
}

func TestContextEmbeddingConfigurationRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{
		`{"backend":"unknown"}`,
		`{"backend":"embeddinggemma2","model_directory":"m","model_file":"../model.onnx","runtime_library":"r"}`,
		`{"backend":"embeddinggemma2","model_directory":"m","model_file":"model.onnx","runtime_library":"r","chunk_tokens":8193}`,
		`{"backend":"embeddinggemma2","model_directory":"m","model_file":"model.onnx","runtime_library":"r","threads":-1}`,
		`{"backend":"embeddinggemma2","model_directory":"m","model_file":"model.onnx","runtime_library":"r","unexpected":true}`,
	} {
		path := filepath.Join(t.TempDir(), "providers.json")
		testutil.RequireNoError(t, os.WriteFile(path, []byte(`{"context_embeddings":`+input+`}`), 0600))
		if _, operationError := LoadContextConfiguration(path); operationError == nil {
			t.Fatalf("accepted invalid settings %s", input)
		}
	}
	path := filepath.Join(t.TempDir(), "missing.json")
	configuration, operationError := LoadContextConfiguration(path)
	testutil.RequireNoError(t, operationError)
	if configuration.Backend != "disabled" {
		t.Fatalf("missing file injected a model: %+v", configuration)
	}
}
