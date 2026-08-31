package evalresolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/evalagentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldefinitionstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldocument"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

var ErrUnresolvable = errors.New("evaluation set is not resolvable")

type agentEvaluationStore interface {
	Get(ctx context.Context, accountID, agentName string) (*evalagentstore.AgentEvaluation, error)
}

type definitionStore interface {
	Get(ctx context.Context, evaluationRef string) (*evaldefinitionstore.Definition, error)
}

type Resolver struct {
	agents      agentEvaluationStore
	definitions definitionStore
}

func NewResolver(agents agentEvaluationStore, definitions definitionStore) *Resolver {
	return &Resolver{agents: agents, definitions: definitions}
}

func (r *Resolver) ActiveRef(ctx context.Context, accountID, agentName string) (string, error) {
	ae, err := r.agents.Get(ctx, accountID, agentName)
	if err != nil {
		return "", fmt.Errorf("evalresolve active ref: %w", err)
	}
	if ae == nil {
		return evalpreset.RefDefaultSet, nil
	}
	return ae.EvaluationRef, nil
}

func (r *Resolver) Set(ctx context.Context, evaluationRef string) ([]evaluator.Evaluator, error) {
	if evalpreset.IsSetRef(evaluationRef) {
		return evalpreset.ResolveSet(evaluationRef)
	}
	def, err := r.definitions.Get(ctx, evaluationRef)
	if err != nil {
		return nil, fmt.Errorf("evalresolve resolve set: %w", err)
	}
	if def == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnresolvable, evaluationRef)
	}
	var doc evaldocument.Document
	if err := json.Unmarshal(def.DefinitionJSON, &doc); err != nil {
		return nil, fmt.Errorf("evalresolve resolve set: unmarshal definition: %w", err)
	}
	return evaldocument.ResolveDocument(doc)
}
