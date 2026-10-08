package loop

import (
	"context"
	"fmt"
	"maps"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
)

type pluginPromptRenderer struct {
	agentLoop *Loop
	templates map[string]*startprompt.Template
}

// PromptRenderer exposes the same registry-backed values as the startup prompt.
// The application owns file loading; plugins receive only this public service.
func (agentLoop *Loop) PromptRenderer(templates map[string]*startprompt.Template) harness.PromptRenderer {
	return &pluginPromptRenderer{agentLoop: agentLoop, templates: maps.Clone(templates)}
}

func (renderer *pluginPromptRenderer) RenderPrompt(operationContext context.Context, session atom.Session, name, model string) (string, error) {
	template, found := renderer.templates[name]
	if !found || template == nil {
		return "", fmt.Errorf("prompt template %q is not available", name)
	}
	return renderer.agentLoop.renderPromptTemplate(harness.WithSession(operationContext, session), session, model, template)
}
