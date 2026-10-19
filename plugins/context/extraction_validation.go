package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func sourceEvidenceFailure(source source, found bool, quote string) string {
	if !found {
		return "source was not supplied"
	}
	if source.Deleted {
		return "source is deleted"
	}
	if quote == "" {
		return "quote is empty"
	}
	if utf8.RuneCountInString(quote) > 600 {
		return "quote exceeds 600 characters"
	}
	if !strings.Contains(source.Text, quote) {
		return "quote is not a literal excerpt of the identified source"
	}
	return ""
}

// A rejected page is durable diagnostic data, not a prepared reduction. Keep
// the original error and the lease/publication fences even if saving it fails.
func (plugin *Plugin) saveRejectedExtraction(operationContext context.Context, workspaceID string, job memoryJob, page int, response harness.WorkspaceAgentResponse, validationError error) error {
	storageError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		stored, operationError := validateJobLease(database, job)
		if operationError != nil {
			return operationError
		}
		stored.ValidationOutput = boundedUTF8(response.Text, 65536)
		if operationError := database.Put("attempt", fmt.Sprintf("%s:%d:%d", job.ID, job.Attempts, page), map[string]any{
			"job_id": job.ID, "model": response.Model, "text": stored.ValidationOutput, "usage": response.Usage,
			"validation_error": validationError.Error(), "page": page,
		}); operationError != nil {
			return operationError
		}
		return database.Put("job", job.ID, stored)
	})
	return errors.Join(validationError, storageError)
}

func (plugin *Plugin) encodeExtractionAttempt(operationContext context.Context, workspaceID string, page extractionInput, job memoryJob, configuration configuration) ([]byte, error) {
	page.PreviousError = boundedUTF8(job.Error, 500)
	if job.ValidationOutput == "" {
		return json.Marshal(page)
	}
	model, operationError := plugin.services.Models.Model(operationContext, workspaceID, configuration.WorkerModel, configuration.WorkerEffort)
	if operationError != nil {
		return nil, operationError
	}
	limit := model.ContextMax - configuration.WorkerOutputTokens - len(extractionPrompt) - 768
	page.PreviousOutput = boundedUTF8(job.ValidationOutput, 16384)
	for {
		encoded, operationError := json.Marshal(page)
		if operationError != nil {
			return nil, operationError
		}
		if len(encoded) <= limit || page.PreviousOutput == "" {
			return encoded, nil
		}
		page.PreviousOutput = boundedUTF8(page.PreviousOutput, len(page.PreviousOutput)/2)
	}
}
