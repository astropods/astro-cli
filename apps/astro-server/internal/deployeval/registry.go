package deployeval

import "github.com/astropods/astro/apps/astro-server/internal/deployer"

// Deps bundles the dependencies an Evaluator factory may need. Add a field
// here, not a new registration signature, when a future evaluator needs
// something an existing one doesn't (e.g. a different store).
type Deps struct {
	Deployer *deployer.Deployer
}

// Factory builds one Evaluator from the shared Deps. Evaluators register a
// Factory from an init() in the file that defines them, so adding a new
// evaluator means adding a file — not also editing main.go's wiring.
type Factory func(Deps) Evaluator

var factories []Factory

// Register adds an evaluator factory to the set BuildAll constructs. Call
// from an init() in the evaluator's own file:
//
//	func init() { Register(func(d Deps) Evaluator { return NewFoo(d.Deployer) }) }
func Register(f Factory) {
	factories = append(factories, f)
}

// BuildAll constructs every registered evaluator, in registration order.
func BuildAll(deps Deps) []Evaluator {
	out := make([]Evaluator, 0, len(factories))
	for _, f := range factories {
		out = append(out, f(deps))
	}
	return out
}
