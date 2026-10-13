package provider

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func inlineMediaDataURI(data []byte, mediaType string) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// Protocol encoders retain their own content shapes but share media bytes,
// defaults and admission checks.
func encodeImageURL(content atom.Content) (string, error) {
	imageURL := content.URL
	if imageURL == "" {
		if len(content.Data) == 0 {
			return "", fmt.Errorf("image content has no data")
		}
		mediaType := content.MIME
		if mediaType == "" {
			mediaType = "image/png"
		}
		imageURL = inlineMediaDataURI(content.Data, mediaType)
	}
	parsed, operationError := url.Parse(imageURL)
	if operationError != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "data") {
		return "", fmt.Errorf("image URL requires HTTP, HTTPS or a data URI")
	}
	return imageURL, nil
}

func encodeFileData(content atom.Content) (string, error) {
	if len(content.Data) == 0 {
		return "", fmt.Errorf("file input requires inline data")
	}
	mediaType := content.MIME
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return inlineMediaDataURI(content.Data, mediaType), nil
}
