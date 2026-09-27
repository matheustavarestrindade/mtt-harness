package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	ID     *int64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		var message request
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.ID == nil {
			continue
		}
		var result any
		switch message.Method {
		case "initialize":
			result = map[string]any{
				"name": "echo",
				"tools": []map[string]any{{
					"name":         "echo",
					"description":  "Echo the text",
					"categories":   []string{"test"},
					"input_schema": map[string]any{"type": "object"},
				}},
			}
		case "tool/run":
			var params struct {
				Input struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(message.Params, &params)
			result = map[string]any{
				"status":  "ok",
				"content": []map[string]any{{"type": "text", "text": "echo: " + params.Input.Text}},
			}
		case "decide":
			result = map[string]any{"kind": "allow"}
		case "pipe":
			var params struct {
				Value json.RawMessage `json:"value"`
			}
			_ = json.Unmarshal(message.Params, &params)
			result = params.Value
		}
		answer, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
		fmt.Fprintf(writer, "%s\n", answer)
		writer.Flush()
	}
}
