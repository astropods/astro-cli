package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// ReferenceKind identifies the category of a ${} reference.
type ReferenceKind string

const (
	RefModel      ReferenceKind = "models"
	RefKnowledge  ReferenceKind = "knowledge"
	RefTool       ReferenceKind = "tools"
	RefCredential ReferenceKind = "credentials"
	RefSource     ReferenceKind = "source"
)

// Reference represents a parsed ${section.name.attribute} reference.
type Reference struct {
	Raw       string        // original string, e.g. "${models.local_llm.host}"
	Kind      ReferenceKind // e.g. RefModel
	Name      string        // e.g. "local_llm" (component name or credential key)
	Attribute string        // e.g. "host", "port", "url" (empty for credentials)
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
// to a declared component, credential, or source attribute.
func ValidateReferences(refs []Reference, ds *AstroDeploymentSpec) []string {
	var errs []string

	for _, ref := range refs {
		switch ref.Kind {
		case RefModel:
			if _, ok := ds.Models[ref.Name]; !ok {
				errs = append(errs, fmt.Sprintf("%s: model %q not declared", ref.Raw, ref.Name))
			} else if !isValidComponentAttr(ref.Attribute) {
				errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected host, port, or url)", ref.Raw, ref.Attribute))
			}

		case RefKnowledge:
			if _, ok := ds.Knowledge[ref.Name]; !ok {
				errs = append(errs, fmt.Sprintf("%s: knowledge store %q not declared", ref.Raw, ref.Name))
			} else if !isValidComponentAttr(ref.Attribute) {
				errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected host, port, or url)", ref.Raw, ref.Attribute))
			}

		case RefTool:
			if _, ok := ds.Tools[ref.Name]; !ok {
				errs = append(errs, fmt.Sprintf("%s: tool %q not declared", ref.Raw, ref.Name))
			} else if !isValidComponentAttr(ref.Attribute) {
				errs = append(errs, fmt.Sprintf("%s: invalid attribute %q (expected host, port, or url)", ref.Raw, ref.Attribute))
			}

		case RefCredential:
			if _, ok := ds.Credentials[ref.Name]; !ok {
				errs = append(errs, fmt.Sprintf("%s: credential %q not declared", ref.Raw, ref.Name))
			}

		case RefSource:
			if ref.Name != "name" && ref.Name != "build" {
				errs = append(errs, fmt.Sprintf("%s: invalid source attribute %q (expected name or build)", ref.Raw, ref.Name))
			}
		}
	}

	return errs
}

// IsReference returns true if the string contains at least one ${} reference.
func IsReference(s string) bool {
	return refPattern.MatchString(s)
}

// IsCredentialReference returns true if the string is a credential reference.
func IsCredentialReference(s string) bool {
	refs := ParseReferences(s)
	return len(refs) == 1 && refs[0].Kind == RefCredential
}

func parseRefInner(raw, inner string) (Reference, error) {
	parts := strings.SplitN(inner, ".", 3)
	if len(parts) < 2 {
		return Reference{}, fmt.Errorf("invalid reference %q: need at least section.name", raw)
	}

	section := parts[0]
	ref := Reference{Raw: raw}

	switch ReferenceKind(section) {
	case RefModel, RefKnowledge, RefTool:
		ref.Kind = ReferenceKind(section)
		ref.Name = parts[1]
		if len(parts) == 3 {
			ref.Attribute = parts[2]
		}
	case RefCredential:
		ref.Kind = RefCredential
		ref.Name = parts[1]
	case RefSource:
		ref.Kind = RefSource
		ref.Name = parts[1]
	default:
		return Reference{}, fmt.Errorf("invalid reference %q: unknown section %q", raw, section)
	}

	return ref, nil
}

func isValidComponentAttr(attr string) bool {
	return attr == "host" || attr == "port" || attr == "url"
}
