package scripts_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativeLauncherContracts(testingHandle *testing.T) {
	if runtime.GOOS != "linux" {
		testingHandle.Skip("the full native launcher uses Linux libraries")
	}
	if _, operationError := exec.LookPath("python3"); operationError != nil {
		testingHandle.Skip("Python is not installed")
	}
	if _, operationError := exec.LookPath("bash"); operationError != nil {
		testingHandle.Skip("Bash is not installed")
	}
	if operationError := exec.Command("python3", "-c", "import sys; raise SystemExit(sys.version_info < (3,10))").Run(); operationError != nil {
		testingHandle.Skip("Python 3.10 or newer is required for native launcher tests")
	}
	_, source, _, found := runtime.Caller(0)
	if !found {
		testingHandle.Fatal("test source path is unavailable")
	}
	command := exec.Command("python3", filepath.Join(filepath.Dir(source), "native_unit_test.py"), "-v")
	if output, operationError := command.CombinedOutput(); operationError != nil {
		testingHandle.Fatalf("native launcher contracts: %v\n%s", operationError, output)
	}
}
