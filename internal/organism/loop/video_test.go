package loop

import (
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestVideoSubmissionAndModelInputGates(test *testing.T) {
	contents := []atom.Content{{Type: atom.Video, Data: []byte("clip"), MIME: "video/mp4", Filename: "clip.mp4"}}
	testutil.RequireNoError(test, validateMessageContent(contents))
	messages := []atom.Message{{Role: atom.RoleUser, Content: contents}}
	testutil.RequireNoError(test, validateModelMedia(atom.ModelInfo{ID: "fixture/video", Input: []atom.MediaType{atom.Text, atom.Video}}, messages))
	if validateModelMedia(atom.ModelInfo{ID: "fixture/text", Input: []atom.MediaType{atom.Text}, Output: []atom.MediaType{atom.Video}}, messages) == nil {
		test.Fatal("output capability incorrectly allowed video input")
	}
	if validateMessageContent([]atom.Content{{Type: atom.Video}}) == nil {
		test.Fatal("empty video accepted")
	}
}
