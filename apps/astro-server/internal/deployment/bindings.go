package deployment

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
)

// ResolvedBinding holds a validated and resolved knowledge store binding.
type ResolvedBinding struct {
	Store    *knowledgestore.KnowledgeStore
	ARN      string
	Name     string
	Provider string
	Status   string
}

// ResolveBindings validates and resolves a map of knowledge entry name → store ARN.
// Returns resolved bindings and any validation errors.
func ResolveBindings(
	ctx context.Context,
	ksStore *knowledgestore.Store,
	accountID string,
	knowledge map[string]DeploymentKnowledge,
	requested map[string]string,
) (map[string]ResolvedBinding, []ValidationError) {
	resolved := make(map[string]ResolvedBinding, len(requested))
	var errs []ValidationError

	for name, arn := range requested {
		field := fmt.Sprintf("bindings.knowledge.%s", name)

		// Entry must exist in the agent's knowledge map.
		entry, ok := knowledge[name]
		if !ok {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("no knowledge entry %q in agent spec", name),
			})
			continue
		}

		// ARN must resolve to a store in the caller's account.
		store, err := ksStore.GetByARN(ctx, arn)
		if err != nil {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "failed to look up store",
			})
			continue
		}
		if store == nil || store.AccountID != accountID {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: "store not found",
			})
			continue
		}

		// Provider must match.
		if store.Provider != entry.Provider {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("provider mismatch: entry declares %s, store is %s", entry.Provider, store.Provider),
			})
			continue
		}

		// Store must be ready.
		if store.Status != knowledgestore.StatusReady {
			errs = append(errs, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("store is not ready (status: %s)", store.Status),
			})
			continue
		}

		resolved[name] = ResolvedBinding{
			Store:    store,
			ARN:      arn,
			Name:     store.Name,
			Provider: store.Provider,
			Status:   store.Status,
		}
	}

	return resolved, errs
}
