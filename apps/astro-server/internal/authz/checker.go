package authz

import "context"

// Checker decides whether a subject may perform an action on a resource.
type Checker interface {
	Authorize(ctx context.Context, sub Subject, action Action, res ResourceRef) (bool, error)
}
