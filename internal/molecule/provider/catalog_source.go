package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const metadataCatalogLimit = 32 * 1024 * 1024

// Metadata sources are public catalogs. Never copy provider authentication
// headers to them, even when they share the provider's HTTP client.
func (standardProvider *Standard) readCatalogSource(operationContext context.Context, source string, byteLimit int64) ([]byte, error) {
	var body io.ReadCloser
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		request, operationError := http.NewRequestWithContext(operationContext, http.MethodGet, source, nil)
		if operationError != nil {
			return nil, operationError
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "mtt-harness/1.0")
		response, operationError := standardProvider.client.Do(request)
		if operationError != nil {
			return nil, operationError
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("catalog source returned HTTP %d", response.StatusCode)
		}
		body = response.Body
	} else {
		file, operationError := os.Open(source)
		if operationError != nil {
			return nil, operationError
		}
		body = file
	}
	defer body.Close()
	data, operationError := io.ReadAll(io.LimitReader(body, byteLimit+1))
	if operationError != nil {
		return nil, operationError
	}
	if int64(len(data)) > byteLimit {
		return nil, fmt.Errorf("catalog source exceeds %d bytes", byteLimit)
	}
	return data, operationContext.Err()
}
