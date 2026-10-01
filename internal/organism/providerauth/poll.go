package providerauth

import (
	"context"
	"errors"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/openaiauth"
)

func (service *Service) poll(operationContext context.Context, flow *loginFlow, challenge openaiauth.DeviceChallenge) {
	defer service.workers.Done()
	defer flow.cancel()
	for {
		timer := time.NewTimer(challenge.Interval)
		select {
		case <-operationContext.Done():
			timer.Stop()
			service.finish(flow, atom.OAuthCredential{}, operationContext.Err())
			return
		case <-timer.C:
		}
		credential, operationError := service.client.Poll(operationContext, challenge)
		if errors.Is(operationError, openaiauth.ErrPending) {
			continue
		}
		if operationError == nil {
			operationError = operationContext.Err()
		}
		service.finish(flow, credential, operationError)
		return
	}
}

func (service *Service) finish(flow *loginFlow, credential atom.OAuthCredential, operationError error) {
	service.gate <- struct{}{}
	defer service.release()
	if service.flow != flow || flow.status.Status != "pending" || service.closed {
		return
	}
	if service.context.Err() != nil {
		operationError = service.context.Err()
	}
	if operationError == nil {
		operationError = service.secrets.SaveOAuthCredential(service.context, flow.status.Provider, credential)
	}
	if operationError == nil {
		flow.status.Status = "connected"
		flow.status.UserCode = ""
		return
	}
	flow.status.Status = "error"
	flow.status.Error = operationError.Error()
	if errors.Is(operationError, context.DeadlineExceeded) {
		flow.status.Status = "expired"
		flow.status.Error = "The OpenAI device code expired. Start sign-in again."
	}
	if errors.Is(operationError, context.Canceled) {
		flow.status.Status = "cancelled"
	}
}
