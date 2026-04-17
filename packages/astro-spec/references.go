package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// ReferenceKind identifies the category of a ${} reference.
type ReferenceKind string

const (
	RefModel       ReferenceKind = "models"
	RefKnowledge   ReferenceKind = "knowledge"
	RefIntegration ReferenceKind = "integrations"
	RefVariable    ReferenceKind = "variables"
	RefSource      ReferenceKind = "source"
)

// Reference represents a parsed ${} reference.
// Component refs can be 3-part (section.name.host) or 4-part (section.name.endpoint.attr).
// Variable refs are 2-part (variables.key).
// Source refs are 2-part (source.attr).
type Reference struct {
	Raw       string        // original string, e.g. "${models.local_llm.http.port}"
	Kind      ReferenceKind // e.g. RefModel
	Name      string        // component name or variable/source key
	Endpoint  string        // endpoint name for 4-part port/url refs (empty for host refs)
	Attribute string        // e.g. "host", "port", "url" (empty for variables)
}

var refPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ParseReferences extracts all ${} references from a string value.
func ParseReferences(value string) []Reference {
	matches := refPattern.FindAllStringSubmatch(value, -1)
	refs := make([]Reference, 0, len(matches))

	for _, match := range matches {
		raw := match[0]
		inner := match[1]

		ref, err := parseRefInner(raw, inner)
		if err != nil {
			continue // skip malformed references; validation catches them
		}
		refs = append(refs, ref)
	}
	return refs
}

// ExtractAllReferences scans all string values in an environment map
// and returns every reference found.
func ExtractAllReferences(env map[string]string) []Reference {
	var all []Reference
	for _, v := range env {
		all = append(all, ParseReferences(v)...)
	}
	return all
}

// ValidateReferences checks that every reference in agent.environment resolves
// to a declared component, variable, or source attribute.
func ValidateReferences(refs []Reference, ds *AstroDeploymentSpec) []string {
	var errs []string

	for _, ref := range refs {
		switch ref.Kind {
		case RefModel:
			m, ok := ds.Models[ref.Name]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: model %q not declared", ref.Raw, ref.Name))
				continue
			}
			if ref.Endpoint == "" {
				// 3-part: only "host" is valid
				if ref.Attribute != "host" {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q for 3-part ref (only \"host\" allowed; use endpoint name for port/url)", ref.Raw, ref.Attribute))
				}
			} else {
				// 4-part: validate endpoint exists and attribute is port/url
				if _, epOK := m.Endpoints[ref.Endpoint]; !epOK {
					errs = append(errs, fmt.Sprintf("%s: endpoint %q not declared on model %q", ref.Raw, ref.Endpoint, ref.Name))
				} else if !isValidEndpointAttr(ref.Attribute) {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected port or url)", ref.Raw, ref.Attribute))
				}
			}

		case RefKnowledge:
			k, ok := ds.Knowledge[ref.Name]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: knowledge store %q not declared", ref.Raw, ref.Name))
				continue
			}
			if ref.Endpoint == "" {
				if ref.Attribute != "host" {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q for 3-part ref (only \"host\" allowed; use endpoint name for port/url)", ref.Raw, ref.Attribute))
				}
			} else {
				if _, epOK := k.Endpoints[ref.Endpoint]; !epOK {
					errs = append(errs, fmt.Sprintf("%s: endpoint %q not declared on knowledge %q", ref.Raw, ref.Endpoint, ref.Name))
				} else if !isValidEndpointAttr(ref.Attribute) {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected port or url)", ref.Raw, ref.Attribute))
				}
			}

		case RefIntegration:
			t, ok := ds.Integrations[ref.Name]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: integration %q not declared", ref.Raw, ref.Name))
				continue
			}
			if ref.Endpoint == "" {
				if ref.Attribute != "host" {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q for 3-part ref (only \"host\" allowed; use endpoint name for port/url)", ref.Raw, ref.Attribute))
				}
			} else {
				if _, epOK := t.Endpoints[ref.Endpoint]; !epOK {
					errs = append(errs, fmt.Sprintf("%s: endpoint %q not declared on integration %q", ref.Raw, ref.Endpoint, ref.Name))
				} else if !isValidEndpointAttr(ref.Attribute) {
					errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected port or url)", ref.Raw, ref.Attribute))
				}
			}

		case RefVariable:
			if _, ok := ds.Variables[ref.Name]; !ok {
				errs = append(errs, fmt.Sprintf("%s: variable %q not declared", ref.Raw, ref.Name))
			}

		case RefSource:
			if ref.Name != "name" && ref.Name != "build" && ref.Name != "account" && ref.Name != "registry" {
				errs = append(errs, fmt.Sprintf("%s: invalid source attribute %q (expected name, build, account, or registry)", ref.Raw, ref.Name))
			}
		}
	}

	return errs
}

// IsReference returns true if the string contains at least one ${} reference.
func IsReference(s string) bool {
	return refPattern.MatchString(s)
}

// IsVariableReference returns true if the string is a variable reference.
func IsVariableReference(s string) bool {
	refs := ParseReferences(s)
	return len(refs) == 1 && refs[0].Kind == RefVariable
}

func parseRefInner(raw, inner string) (Reference, error) {
	// Split into at most 4 parts to handle: section.name[.endpoint[.attr]]
	parts := strings.SplitN(inner, ".", 4)
	if len(parts) < 2 {
		return Reference{}, fmt.Errorf("invalid reference %q: need at least section.name", raw)
	}

	section := parts[0]
	ref := Reference{Raw: raw}

	switch ReferenceKind(section) {
	case RefModel, RefKnowledge, RefIntegration:
		ref.Kind = ReferenceKind(section)
		ref.Name = parts[1]
		if len(parts) == 3 {
			// 3-part: section.name.attribute (only "host" is valid)
			ref.Attribute = parts[2]
		} else if len(parts) == 4 {
			// 4-part: section.name.endpoint.attribute
			ref.Endpoint = parts[2]
			ref.Attribute = parts[3]
		}
		// 2-part (section.name) is also valid, attribute left empty
	case RefVariable:
		ref.Kind = RefVariable
		ref.Name = parts[1]
	case RefSource:
		ref.Kind = RefSource
		ref.Name = parts[1]
	default:
		return Reference{}, fmt.Errorf("invalid reference %q: unknown section %q", raw, section)
	}

	return ref, nil
}

// isValidEndpointAttr returns true for valid 4-part endpoint attributes.
func isValidEndpointAttr(attr string) bool {
	return attr == "port" || attr == "url"
}
