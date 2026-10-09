package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

// These wire shapes are documented by OpenRouter's compatible API:
// https://openrouter.ai/docs/guides/overview/multimodal/videos
func TestVideoRequestsPreserveInlineBytesAndUseDocumentedWireTypes(test *testing.T) {
	videoData := []byte{0, 0, 0, 20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 255, 128, 1}
	for _, protocol := range []string{"chat_completions", "responses"} {
		test.Run(protocol, func(test *testing.T) {
			captured := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				var payload map[string]any
				if json.NewDecoder(request.Body).Decode(&payload) != nil {
					http.Error(responseWriter, "invalid JSON", 400)
					return
				}
				captured <- payload
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				if protocol == "responses" {
					fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
					return
				}
				fmt.Fprint(responseWriter, "data: [DONE]\n\n")
			}))
			defer server.Close()
			operationContext, cancel := context.WithTimeout(context.Background(), time.Second*3)
			defer cancel()
			modelProvider := New(atom.ProviderSpec{Name: "fixture-video-provider", Protocol: protocol, APIURL: server.URL, Authentication: "none"})
			stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "fixture-video-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Describe the clip"}, {Type: atom.Video, Data: videoData, MIME: "video/mp4", Filename: "clip.mp4"}}}}})
			testutil.RequireNoError(test, operationError)
			for {
				_, operationError = stream.Recv(operationContext)
				if operationError == io.EOF {
					break
				}
				testutil.RequireNoError(test, operationError)
			}
			payload := <-captured
			field := "messages"
			if protocol == "responses" {
				field = "input"
			}
			parts := payload[field].([]any)[0].(map[string]any)["content"].([]any)
			part := parts[1].(map[string]any)
			var encoded string
			if protocol == "responses" {
				if part["type"] != "input_video" {
					test.Fatalf("video part: %#v", part)
				}
				encoded = part["video_url"].(string)
			} else {
				if part["type"] != "video_url" {
					test.Fatalf("video part: %#v", part)
				}
				encoded = part["video_url"].(map[string]any)["url"].(string)
			}
			if !strings.HasPrefix(encoded, "data:video/mp4;base64,") {
				test.Fatalf("video MIME was lost: %s", encoded)
			}
			decoded, operationError := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "data:video/mp4;base64,"))
			testutil.RequireNoError(test, operationError)
			if !bytes.Equal(decoded, videoData) {
				test.Fatal("video bytes changed in provider request")
			}
		})
	}
}

func TestVideoURLValidationAndCatalogMetadata(test *testing.T) {
	for _, content := range []atom.Content{{Type: atom.Video}, {Type: atom.Video, Data: []byte("video"), MIME: "image/png"}, {Type: atom.Video, URL: "file:///private/clip.mp4"}, {Type: atom.Video, URL: "javascript:alert(1)"}, {Type: atom.Video, URL: "data:text/html;base64,WA=="}, {Type: atom.Video, URL: "data:video/mp4;base64,!!!"}, {Type: atom.Video, URL: "https://user:secret@example.com/clip.mp4"}} {
		if _, operationError := encodeVideoURL(content); operationError == nil {
			test.Fatalf("invalid video accepted: %#v", content)
		}
	}
	remote := "https://example.com/clip.mp4"
	encoded, operationError := encodeVideoURL(atom.Content{Type: atom.Video, URL: remote})
	testutil.RequireNoError(test, operationError)
	if encoded != remote {
		test.Fatal("remote video URL changed")
	}
	modelInput := catalogMediaTypes([]string{"text", "video", "pdf", "video"})
	if len(modelInput) != 3 || modelInput[1] != atom.Video || modelInput[2] != atom.File {
		test.Fatalf("media catalog: %#v", modelInput)
	}
	testutil.RequireNoError(test, validateModelMetadata(ModelMetadata{Input: modelInput}))
}
