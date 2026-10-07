//go:build gemma && cgo

package embeddinggemma

import (
	"errors"
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// The binding owns a process-global C environment. Reference counting prevents
// an encoder from unloading the library while another session still uses it.
var nativeEnvironment struct {
	sync.Mutex
	users   int
	library string
}

func acquireEnvironment(library string) error {
	nativeEnvironment.Lock()
	defer nativeEnvironment.Unlock()
	if nativeEnvironment.users > 0 {
		if nativeEnvironment.library != library {
			return fmt.Errorf("EmbeddingGemma native runtime is already loaded from another path")
		}
		nativeEnvironment.users++
		return nil
	}
	if ort.IsInitialized() {
		return fmt.Errorf("EmbeddingGemma native runtime is owned by another service")
	}
	ort.SetSharedLibraryPath(library)
	if operationError := ort.InitializeEnvironment(); operationError != nil {
		return operationError
	}
	if operationError := ort.DisableTelemetry(); operationError != nil {
		return errors.Join(operationError, ort.DestroyEnvironment())
	}
	nativeEnvironment.library = library
	nativeEnvironment.users = 1
	return nil
}

func releaseEnvironment() error {
	nativeEnvironment.Lock()
	defer nativeEnvironment.Unlock()
	nativeEnvironment.users--
	if nativeEnvironment.users > 0 {
		return nil
	}
	nativeEnvironment.library = ""
	return ort.DestroyEnvironment()
}
