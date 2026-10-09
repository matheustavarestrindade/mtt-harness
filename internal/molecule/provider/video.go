package provider

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Video-capable compatible endpoints use video_url in Chat Completions and
// input_video in Responses. Model input metadata gates this provider extension.
func encodeVideoURL(content atom.Content) (string, error) {
	if content.MIME != "" {
		if _, operationError := parseVideoMIME(content.MIME); operationError != nil {
			return "", operationError
		}
	}
	if content.URL != "" {
		parsed, operationError := url.Parse(content.URL)
		if operationError != nil || parsed.User != nil {
			return "", fmt.Errorf("video URL is invalid")
		}
		switch parsed.Scheme {
		case "http", "https":
			if parsed.Host == "" {
				return "", fmt.Errorf("video URL has no host")
			}
			return content.URL, nil
		case "data":
			separator := strings.IndexByte(content.URL, ',')
			if separator < 0 || !strings.HasSuffix(content.URL[:separator], ";base64") {
				return "", fmt.Errorf("video data URL requires base64 data")
			}
			mediaType := strings.TrimSuffix(strings.TrimPrefix(content.URL[:separator], "data:"), ";base64")
			if _, operationError := parseVideoMIME(mediaType); operationError != nil {
				return "", operationError
			}
			decoded, decodeError := base64.StdEncoding.DecodeString(content.URL[separator+1:])
			if decodeError != nil || len(decoded) == 0 {
				return "", fmt.Errorf("video data URL contains invalid or empty base64 data")
			}
			return content.URL, nil
		default:
			return "", fmt.Errorf("video URL requires HTTP, HTTPS or a video data URI")
		}
	}
	if len(content.Data) == 0 {
		return "", fmt.Errorf("video content has no data")
	}
	mediaType, operationError := parseVideoMIME(content.MIME)
	if operationError != nil {
		return "", operationError
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(content.Data), nil
}

func parseVideoMIME(value string) (string, error) {
	mediaType, _, operationError := mime.ParseMediaType(value)
	if operationError != nil || !strings.HasPrefix(mediaType, "video/") || mediaType == "video/" || strings.Contains(mediaType, "*") {
		return "", fmt.Errorf("video input requires a video MIME type")
	}
	return mediaType, nil
}
