package spec

// Package envresolver implements the environment variable injection model defined in
// spec sections 8.1–8.5. It is the single source of truth for computing which env var
// keys are injected into each container, and what values they hold.
//
// Rules summary:
//   8.1  Cloud providers   → {UPPER(provider)}_{suffix} injected into the agent.
//        Duplicate entries → qualified with entry name; primary also gets bare key.
//   8.2  Self-hosted providers → {EnvPrefix}_{HOST/PORT/URL} injected into the agent.
//        Model providers additionally inject {EnvPrefix}_{BASE_URL} and {EnvPrefix}_{MODEL}.
//   8.3  Container-mode entries → {SECTION}_{UPPER(name)}_{HOST/PORT/URL}.
//   8.4  Inputs → name used directly as env var key in the target container.
//        Top-level inputs → all containers. Component inputs → their container only.
//   8.5  Name sanitization: lower → replace -, _ and . with _ → strip non-alnum → upper.

import (
	"regexp"
	"sort"
	"strings"
)

// ─── Name sanitization (spec §8.5) ───────────────────────────────────────────

var (
	sanitizeNonAlnum        = regexp.MustCompile(`[^a-z0-9_]`)
	sanitizeMultiUnderscore = regexp.MustCompile(`_+`)
)

// SanitizeEnvName sanitizes an entry name and returns the uppercased form ready for
// use inside an env var key. Implements spec section 8.5.
//
// Steps: lowercase → replace -, _ and . with _ → remove non-alphanumeric →
// collapse consecutive underscores → trim leading/trailing underscores → uppercase.
//
// Examples:
//
//	"my_model"  → "MY_MODEL"
//	"my.store"  → "MY_STORE"
//	"my-model"  → "MY_MODEL"
//	"llm"       → "LLM"
//	"local_llm" → "LOCAL_LLM"
func SanitizeEnvName(name string) string {
	s := strings.ToLower(name)
	s = strings.NewReplacer("-", "_", ".", "_").Replace(s)
	s = sanitizeNonAlnum.ReplaceAllString(s, "")
	s = sanitizeMultiUnderscore.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return strings.ToUpper(s)
}

// ─── Connection address ───────────────────────────────────────────────────────

// ConnectionAddress holds the resolved connection details for a single component.
// Values may be concrete strings (docker DNS, k8s service DNS) or placeholder
// references (e.g. "${models.llm.host}") for deferred resolution.
type ConnectionAddress struct {
	Host    string
	Port    string
	URL     string
	BaseURL string // URL + "/api"; populated for model providers
}

// ─── Result types ─────────────────────────────────────────────────────────────

// EnvResult holds the computed env var maps for every container.
type EnvResult struct {
	Agent     map[string]string
	Models    map[string]map[string]string
	Knowledge map[string]map[string]string
	Tools     map[string]map[string]string
	Ingestion map[string]map[string]string
}

// CredentialMeta describes one required credential.
type CredentialMeta struct {
	Provider    string
	Category    string // "model", "knowledge", "tool", "provider"
	Description string
	Optional    bool
}

// ─── Public resolver entry points ─────────────────────────────────────────────

// ResolveEnvVars computes the complete env var injection for all containers in a spec.
//
// Parameters:
//   - addrs: maps "models.{name}", "knowledge.{name}", "tools.{name}" to connection
//     details. Values may be concrete strings or deferred placeholders.
//   - credentials: maps credential key → value (cloud + custom-provider secrets).
//   - inputValues: maps input name → user-supplied value (falls back to Input.Default).
//
// Returns an EnvResult with one map per container.
func ResolveEnvVars(s *AstroSpec, addrs map[string]ConnectionAddress, credentials, inputValues map[string]string) EnvResult {
	res := EnvResult{
		Agent:     make(map[string]string),
		Models:    make(map[string]map[string]string),
		Knowledge: make(map[string]map[string]string),
		Tools:     make(map[string]map[string]string),
		Ingestion: make(map[string]map[string]string),
	}

	// §8.1 / §8.2 / §8.3 — connection wiring into agent
	resolveModelConnections(s, addrs, res.Agent)
	resolveKnowledgeConnections(s, addrs, res.Agent)
	resolveToolConnections(s, addrs, res.Agent)

	// §8.1 — credentials (cloud + custom provider secrets) into agent
	for k, v := range credentials {
		res.Agent[k] = v
	}

	// §8.4 — top-level inputs → all containers
	for _, inp := range sortedInputs(s.Inputs) {
		v := resolveInputValue(inp, inputValues)
		if v == "" {
			continue
		}
		res.Agent[inp.Name] = v
		for name := range s.Models {
			ensureComponentMap(res.Models, name)[inp.Name] = v
		}
		for name := range s.Knowledge {
			ensureComponentMap(res.Knowledge, name)[inp.Name] = v
		}
		for name := range s.Tools {
			ensureComponentMap(res.Tools, name)[inp.Name] = v
		}
		for name := range s.Ingestion {
			ensureComponentMap(res.Ingestion, name)[inp.Name] = v
		}
	}

	// §8.4 — agent-specific inputs
	for _, inp := range s.Agent.Inputs {
		if v := resolveInputValue(inp, inputValues); v != "" {
			res.Agent[inp.Name] = v
		}
	}

	// §8.4 — component-specific inputs
	for name, model := range s.Models {
		for _, inp := range model.Inputs {
			if v := resolveInputValue(inp, inputValues); v != "" {
				ensureComponentMap(res.Models, name)[inp.Name] = v
			}
		}
	}
	for name, k := range s.Knowledge {
		for _, inp := range k.Inputs {
			if v := resolveInputValue(inp, inputValues); v != "" {
				ensureComponentMap(res.Knowledge, name)[inp.Name] = v
			}
		}
	}
	for name, t := range s.Tools {
		for _, inp := range t.Inputs {
			if v := resolveInputValue(inp, inputValues); v != "" {
				ensureComponentMap(res.Tools, name)[inp.Name] = v
			}
		}
	}
	for name, ing := range s.Ingestion {
		for _, inp := range ing.Inputs {
			if v := resolveInputValue(inp, inputValues); v != "" {
				ensureComponentMap(res.Ingestion, name)[inp.Name] = v
			}
		}
	}

	return res
}

// CloudCredentialKeys returns the env var key names and metadata for all cloud
// provider credentials derived from the spec (sections 8.1). The returned map is
// keyed by env var key (e.g. "ANTHROPIC_API_KEY").
//
// This is the authoritative implementation of the duplicate-handling rule:
//   - One entry for a provider → bare key {UPPER(provider)}_{suffix}.
//   - Multiple entries for the same provider → qualified keys for all; the primary
//     entry (name matches provider, else first alphabetically) also gets the bare key.
//   - When entry name == provider name, the redundant qualified form is omitted.
func CloudCredentialKeys(s *AstroSpec) map[string]CredentialMeta {
	result := make(map[string]CredentialMeta)

	type cloudEntry struct {
		name     string
		provider string
		category string
		suffixes []CredentialSuffix
	}

	groups := make(map[string][]cloudEntry) // provider → entries

	for name, m := range s.Models {
		if m.IsProviderMode() {
			if _, isCustom := s.Providers[m.Provider]; isCustom {
				continue
			}
			if suffixes, ok := GetCloudModelCredentials(m.Provider); ok {
				p := strings.ToLower(m.Provider)
				groups[p] = append(groups[p], cloudEntry{name, p, "model", suffixes})
			}
		}
	}
	for name, k := range s.Knowledge {
		if k.IsProviderMode() {
			if _, isCustom := s.Providers[k.Provider]; isCustom {
				continue
			}
			if suffixes, ok := GetCloudKnowledgeCredentials(k.Provider); ok {
				p := strings.ToLower(k.Provider)
				groups[p] = append(groups[p], cloudEntry{name, p, "knowledge", suffixes})
			}
		}
	}
	for name, t := range s.Tools {
		if t.IsProviderMode() {
			if _, isCustom := s.Providers[t.Provider]; isCustom {
				continue
			}
			if suffixes, ok := GetCloudToolCredentials(t.Provider); ok {
				p := strings.ToLower(t.Provider)
				groups[p] = append(groups[p], cloudEntry{name, p, "tool", suffixes})
			}
		}
	}

	for _, entries := range groups {
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		isDup := len(entries) > 1
		basePrefix := strings.ToUpper(entries[0].provider)

		// Determine primary entry: prefer name that matches provider.
		// -1 means no match found; bare key is only emitted when a match exists.
		bareIdx := -1
		if isDup {
			for i, e := range entries {
				if strings.EqualFold(e.name, e.provider) {
					bareIdx = i
					break
				}
			}
		}

		for i, e := range entries {
			for _, cs := range e.suffixes {
				if !isDup {
					// Single entry: bare key only.
					result[basePrefix+"_"+cs.Suffix] = CredentialMeta{
						Provider: e.provider, Category: e.category,
						Description: cs.Description, Optional: cs.Optional,
					}
				} else {
					// Qualified key for all entries, unless name == provider (redundant).
					if !strings.EqualFold(e.name, e.provider) {
						result[basePrefix+"_"+SanitizeEnvName(e.name)+"_"+cs.Suffix] = CredentialMeta{
							Provider: e.provider, Category: e.category,
							Description: cs.Description, Optional: cs.Optional,
						}
					}
					// Bare key for primary entry.
					if i == bareIdx {
						result[basePrefix+"_"+cs.Suffix] = CredentialMeta{
							Provider: e.provider, Category: e.category,
							Description: cs.Description, Optional: cs.Optional,
						}
					}
				}
			}
		}
	}

	return result
}

// CustomProviderCredentialKeys returns the env var key names for all custom provider
// variables that are marked secret=true and are referenced by at least one component.
//
// Keys follow §8.1: {UPPER(provider)}_{varName}, where varName is the variable suffix.
// Duplicate-entry handling mirrors §8.1: multiple entries referencing the same custom
// provider produce qualified keys; the primary entry also gets the bare key.
func CustomProviderCredentialKeys(s *AstroSpec) map[string]CredentialMeta {
	result := make(map[string]CredentialMeta)

	type customEntry struct {
		entryName string
		variables []Input
	}
	groups := make(map[string][]customEntry) // provider name → entries

	for name, m := range s.Models {
		if _, ok := s.Providers[m.Provider]; ok {
			groups[m.Provider] = append(groups[m.Provider], customEntry{name, s.Providers[m.Provider].Variables})
		}
	}
	for name, k := range s.Knowledge {
		if _, ok := s.Providers[k.Provider]; ok {
			groups[k.Provider] = append(groups[k.Provider], customEntry{name, s.Providers[k.Provider].Variables})
		}
	}
	for name, t := range s.Tools {
		if _, ok := s.Providers[t.Provider]; ok {
			groups[t.Provider] = append(groups[t.Provider], customEntry{name, s.Providers[t.Provider].Variables})
		}
	}

	for provName, entries := range groups {
		sort.Slice(entries, func(i, j int) bool { return entries[i].entryName < entries[j].entryName })
		isDup := len(entries) > 1
		basePrefix := SanitizeEnvName(provName)

		// -1 means no match found; bare key is only emitted when a match exists.
		bareIdx := -1
		if isDup {
			for i, e := range entries {
				if strings.EqualFold(e.entryName, provName) {
					bareIdx = i
					break
				}
			}
		}

		for i, e := range entries {
			for _, v := range e.variables {
				if !v.Secret {
					continue
				}
				meta := CredentialMeta{
					Provider: provName, Category: "provider",
					Description: v.Description, Optional: v.Optional,
				}
				if !isDup {
					result[basePrefix+"_"+v.Name] = meta
				} else {
					if !strings.EqualFold(e.entryName, provName) {
						result[basePrefix+"_"+SanitizeEnvName(e.entryName)+"_"+v.Name] = meta
					}
					if i == bareIdx {
						result[basePrefix+"_"+v.Name] = meta
					}
				}
			}
		}
	}
	return result
}

// AgentConnectionKeys returns all env var keys that will be auto-injected into the
// agent for component connection wiring (§8.2 and §8.3). Keys are populated using the
// provided addrs map so that the caller controls whether values are concrete or deferred.
// Credential and input keys are NOT included — use CloudCredentialKeys for those.
func AgentConnectionKeys(s *AstroSpec, addrs map[string]ConnectionAddress) map[string]string {
	result := make(map[string]string)
	resolveModelConnections(s, addrs, result)
	resolveKnowledgeConnections(s, addrs, result)
	resolveToolConnections(s, addrs, result)
	return result
}

// AgentKeysForComponent returns the env var key names that one specific component
// contributes to the agent's environment, correctly handling duplicate-provider
// naming by evaluating within the full spec context.
//
// section must be "models", "knowledge", or "tools". entryName is the map key.
// Only connection keys are returned (not model-name keys like OLLAMA_MODEL).
func AgentKeysForComponent(s *AstroSpec, section, entryName string) []string {
	const sentinel = "\x00SENTINEL\x00"
	addrs := map[string]ConnectionAddress{
		section + "." + entryName: {
			Host:    sentinel,
			Port:    sentinel,
			URL:     sentinel,
			BaseURL: sentinel,
		},
	}
	env := AgentConnectionKeys(s, addrs)

	var keys []string
	for k, v := range env {
		if v == sentinel {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// AllCredentialKeys returns all credential key names (cloud + custom provider secrets)
// that will be injected into the agent, with their metadata.
func AllCredentialKeys(s *AstroSpec) map[string]CredentialMeta {
	all := CloudCredentialKeys(s)
	for k, v := range CustomProviderCredentialKeys(s) {
		all[k] = v
	}
	return all
}

// AgentEnvMeta describes one env var automatically injected into the agent.
type AgentEnvMeta struct {
	Source   string // "connection" or "credential"
	Provider string // originating provider name (lowercase), e.g. "qdrant", "anthropic"
	Category string // "model", "knowledge", "tool", "provider"
	Optional bool   // credentials only
}

// AllAgentAutoEnvKeys returns all env var keys automatically injected into the agent:
// connection wiring keys (§8.2/§8.3) and credential keys (§8.1, cloud + custom provider
// secrets). Inputs are not included as they are user-defined.
func AllAgentAutoEnvKeys(s *AstroSpec) map[string]AgentEnvMeta {
	result := make(map[string]AgentEnvMeta)

	for k := range AgentConnectionKeys(s, nil) {
		provider, category := connectionKeySource(s, k)
		result[k] = AgentEnvMeta{Source: "connection", Provider: provider, Category: category}
	}
	for k, meta := range AllCredentialKeys(s) {
		result[k] = AgentEnvMeta{
			Source:   "credential",
			Provider: meta.Provider,
			Category: meta.Category,
			Optional: meta.Optional,
		}
	}
	return result
}

// connectionKeySource returns the provider name and section category for a
// connection env var key by matching against the spec's self-hosted components.
func connectionKeySource(s *AstroSpec, key string) (provider, category string) {
	for name, m := range s.Models {
		if p, ok := LookupBuiltin("models", m.Provider); ok && p.EnvPrefix != "" {
			if strings.HasPrefix(key, p.EnvPrefix+"_") {
				return m.Provider, "model"
			}
		}
		// §8.3 container-mode model (no builtin EnvPrefix).
		if strings.HasPrefix(key, "MODEL_"+SanitizeEnvName(name)+"_") {
			return name, "model"
		}
	}
	for name, k := range s.Knowledge {
		if p, ok := LookupBuiltin("knowledge", k.Provider); ok && p.EnvPrefix != "" {
			if strings.HasPrefix(key, p.EnvPrefix+"_") {
				return k.Provider, "knowledge"
			}
		}
		// §8.3 container-mode knowledge (no builtin EnvPrefix).
		if strings.HasPrefix(key, "KNOWLEDGE_"+SanitizeEnvName(name)+"_") {
			return name, "knowledge"
		}
	}
	for name, t := range s.Tools {
		if strings.HasPrefix(key, "INTEGRATION_"+SanitizeEnvName(name)+"_") {
			prov := t.Provider
			if prov == "" {
				prov = name
			}
			return prov, "tool"
		}
	}
	return "", ""
}

// ─── Internal connection resolution ──────────────────────────────────────────

// resolveModelConnections applies §8.2 (self-hosted) and §8.3 (container) rules
// for all model entries into dst.
func resolveModelConnections(s *AstroSpec, addrs map[string]ConnectionAddress, dst map[string]string) {
	if len(s.Models) == 0 {
		return
	}

	// Count how many entries share each EnvPrefix (for duplicate-key logic).
	prefixCount := make(map[string]int)
	for _, m := range s.Models {
		if m.IsProviderMode() && m.DeploysContainer(s.Providers) {
			if prov := GetModelProvider(m.Provider); prov.EnvPrefix != "" {
				prefixCount[prov.EnvPrefix]++
			}
		}
	}

	names := sortedKeys(s.Models)
	prefixFirst := make(map[string]bool)

	for _, name := range names {
		model := s.Models[name]
		if !model.DeploysContainer(s.Providers) {
			continue // cloud or custom provider — no connection wiring
		}
		addr := addrs["models."+name]

		if model.IsProviderMode() {
			// §8.2 — self-hosted model provider.
			prov := GetModelProvider(model.Provider)
			if prov.EnvPrefix == "" {
				continue
			}
			isDup := prefixCount[prov.EnvPrefix] > 1
			isFirst := !prefixFirst[prov.EnvPrefix]
			prefixFirst[prov.EnvPrefix] = true

			for _, key := range qualifiedKeys(prov.EnvPrefix, name, "HOST", isDup, isFirst) {
				dst[key] = addr.Host
			}
			for _, key := range qualifiedKeys(prov.EnvPrefix, name, "PORT", isDup, isFirst) {
				dst[key] = addr.Port
			}
			for _, key := range qualifiedKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
				dst[key] = addr.URL
			}
			for _, key := range qualifiedKeys(prov.EnvPrefix, name, "BASE_URL", isDup, isFirst) {
				dst[key] = addr.BaseURL
			}
			if model.Model != "" {
				for _, key := range qualifiedKeys(prov.EnvPrefix, name, "MODEL", isDup, isFirst) {
					dst[key] = model.Model
				}
			}
		} else {
			// §8.3 — container-mode model.
			prefix := "MODEL_" + SanitizeEnvName(name)
			dst[prefix+"_HOST"] = addr.Host
			dst[prefix+"_PORT"] = addr.Port
			dst[prefix+"_URL"] = addr.URL
		}
	}
}

// resolveKnowledgeConnections applies §8.2 (self-hosted) and §8.3 (container) rules
// for all knowledge entries into dst.
func resolveKnowledgeConnections(s *AstroSpec, addrs map[string]ConnectionAddress, dst map[string]string) {
	if len(s.Knowledge) == 0 {
		return
	}

	prefixCount := make(map[string]int)
	for _, k := range s.Knowledge {
		if k.IsProviderMode() && k.DeploysContainer(s.Providers) {
			if prov := GetProvider(k.Provider); prov.EnvPrefix != "" {
				prefixCount[prov.EnvPrefix]++
			}
		}
	}

	names := sortedKeys(s.Knowledge)
	prefixFirst := make(map[string]bool)

	for _, name := range names {
		k := s.Knowledge[name]
		if !k.DeploysContainer(s.Providers) {
			continue // cloud or custom provider — no connection wiring
		}
		addr := addrs["knowledge."+name]

		if k.IsProviderMode() {
			// §8.2 — self-hosted knowledge provider.
			prov := GetProvider(k.Provider)
			if prov.EnvPrefix != "" {
				isDup := prefixCount[prov.EnvPrefix] > 1
				isFirst := !prefixFirst[prov.EnvPrefix]
				prefixFirst[prov.EnvPrefix] = true

				for _, key := range qualifiedKeys(prov.EnvPrefix, name, "HOST", isDup, isFirst) {
					dst[key] = addr.Host
				}
				for _, key := range qualifiedKeys(prov.EnvPrefix, name, "PORT", isDup, isFirst) {
					dst[key] = addr.Port
				}
				if prov.URLScheme != "" {
					for _, key := range qualifiedKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
						dst[key] = addr.URL
					}
				}
				continue
			}
		}

		// §8.3 — container-mode knowledge (or provider with no EnvPrefix).
		prefix := "KNOWLEDGE_" + SanitizeEnvName(name)
		dst[prefix+"_HOST"] = addr.Host
		dst[prefix+"_PORT"] = addr.Port
	}
}

// resolveToolConnections applies §8.3 rules for all container-mode tool entries.
func resolveToolConnections(s *AstroSpec, addrs map[string]ConnectionAddress, dst map[string]string) {
	for name, t := range s.Tools {
		if !t.DeploysContainer(s.Providers) {
			continue // cloud or custom provider — no connection wiring
		}
		addr := addrs["tools."+name]
		prefix := "INTEGRATION_" + SanitizeEnvName(name)
		dst[prefix+"_HOST"] = addr.Host
		dst[prefix+"_PORT"] = addr.Port
		dst[prefix+"_URL"] = addr.URL
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// qualifiedKeys returns the env var keys for a single component+suffix, applying the
// duplicate-handling rule from §8.1/§8.2.
//
//   - Not a duplicate: returns ["{prefix}_{suffix}"].
//   - Duplicate, first: returns ["{prefix}_{ENTRY}_{suffix}", "{prefix}_{suffix}"].
//   - Duplicate, not first: returns ["{prefix}_{ENTRY}_{suffix}"].
func qualifiedKeys(prefix, entryName, suffix string, isDup, isFirst bool) []string {
	if !isDup {
		return []string{prefix + "_" + suffix}
	}
	qualified := prefix + "_" + SanitizeEnvName(entryName) + "_" + suffix
	if isFirst {
		return []string{qualified, prefix + "_" + suffix}
	}
	return []string{qualified}
}

func resolveInputValue(inp Input, provided map[string]string) string {
	if v, ok := provided[inp.Name]; ok {
		return v
	}
	return inp.Default
}

func ensureComponentMap(m map[string]map[string]string, name string) map[string]string {
	if m[name] == nil {
		m[name] = make(map[string]string)
	}
	return m[name]
}

// sortedKeys returns map keys sorted alphabetically (for deterministic iteration).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedInputs returns the values of a map[string]Input sorted by key.
func sortedInputs(m map[string]Input) []Input {
	keys := sortedKeys(m)
	out := make([]Input, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}
