package provider

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func encodeMessages(messages []atom.Message) ([]map[string]any, error) {
	var result []map[string]any
	var attachments []atom.Content
	flushAttachments := func() error {
		if len(attachments) == 0 {
			return nil
		}
		content, operationError := encodeChatMessageContent(attachments)
		if operationError != nil {
			return operationError
		}
		result = append(result, map[string]any{"role": "user", "content": content})
		attachments = nil
		return nil
	}
	for _, message := range messages {
		if message.Role != atom.RoleTool {
			if operationError := flushAttachments(); operationError != nil {
				return nil, operationError
			}
		}
		item := map[string]any{"role": string(message.Role)}
		switch message.Role {
		case atom.RoleTool, atom.RoleAssistant:
			item["content"] = concatenateContentText(message.Content)
			if message.Role == atom.RoleTool {
				item["tool_call_id"] = message.ToolCallID
			}
			for _, content := range message.Content {
				if content.Type == atom.Text {
					continue
				}
				if message.Role == atom.RoleAssistant && content.Type == atom.Audio && content.AudioID != "" {
					item["audio"] = map[string]any{"id": content.AudioID}
					continue
				}
				if len(attachments) == 0 {
					attachments = append(attachments, atom.Content{Type: atom.Text, Text: "Media returned by the preceding assistant or tool response:"})
				}
				attachments = append(attachments, content)
			}
			if len(message.ToolCalls) > 0 {
				var calls []map[string]any
				for _, call := range message.ToolCalls {
					calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(call.Input)}})
				}
				item["tool_calls"] = calls
			}
		default:
			content, operationError := encodeChatMessageContent(message.Content)
			if operationError != nil {
				return nil, operationError
			}
			item["content"] = content
		}
		result = append(result, item)
	}
	if operationError := flushAttachments(); operationError != nil {
		return nil, operationError
	}
	return result, nil
}

// Chat Completions only permits text in tool messages. Media is retained and
// sent after the complete group of tool replies, never between paired replies.
func encodeChatMessageContent(contents []atom.Content) (any, error) {
	hasMedia := false
	for _, content := range contents {
		if content.Type != atom.Text {
			hasMedia = true
		}
	}
	if !hasMedia {
		return concatenateContentText(contents), nil
	}
	var parts []map[string]any
	for _, content := range contents {
		switch content.Type {
		case atom.Text:
			parts = append(parts, map[string]any{"type": "text", "text": content.Text})
		case atom.Image:
			imageURL := content.URL
			if imageURL != "" {
				parsed, operationError := url.Parse(imageURL)
				if operationError != nil || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "data") {
					return nil, fmt.Errorf("image URL requires HTTP, HTTPS or a data URI")
				}
			}
			if imageURL == "" {
				if len(content.Data) == 0 {
					return nil, fmt.Errorf("image content has no data")
				}
				mime := content.MIME
				if mime == "" {
					mime = "image/png"
				}
				imageURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content.Data)
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}})
		case atom.Audio:
			format := ""
			switch content.MIME {
			case "audio/wav", "audio/x-wav":
				format = "wav"
			case "audio/mpeg", "audio/mp3":
				format = "mp3"
			}
			if format == "" || len(content.Data) == 0 {
				return nil, fmt.Errorf("audio input requires WAV or MP3 data")
			}
			parts = append(parts, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(content.Data), "format": format}})
		case atom.File:
			if len(content.Data) == 0 {
				return nil, fmt.Errorf("file input requires inline data")
			}
			filename := content.Filename
			if filename == "" {
				filename = "attachment.bin"
				if content.MIME == "application/pdf" {
					filename = "attachment.pdf"
				}
			}
			mime := content.MIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			parts = append(parts, map[string]any{"type": "file", "file": map[string]any{"filename": filename, "file_data": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content.Data)}})
		default:
			return nil, fmt.Errorf("unsupported content type %q", content.Type)
		}
	}
	return parts, nil
}

func concatenateContentText(contents []atom.Content) string {
	var text []string
	for _, content := range contents {
		if content.Type == atom.Text {
			text = append(text, content.Text)
		}
	}
	return strings.Join(text, "\n")
}
