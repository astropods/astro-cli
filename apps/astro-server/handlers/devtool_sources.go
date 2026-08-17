package handlers

// Dev-tool spend (Claude Code, …) reaches Insights through the same fact table
// as agent spend, so a tool needs no read path of its own. This registry is what
// names and brands it: one entry gives it a Sources-filter option, a synthetic
// agents-table row, and chips on the People rows.

// devtoolAdapter registers one coding tool. Key is both the astro.source value
// and the Langfuse trace tag.
type devtoolAdapter struct {
	Key   string
	Label string
	Icon  string // integration-icon key → themed logo (e.g. "anthropic")
}

var devtoolAdapters = []devtoolAdapter{
	{Key: "claude-code", Label: "Claude Code", Icon: "anthropic"},
}

// devtoolIdentity is the row identity for a dev-tool source: system-kind, brand
// icon, and not clickable, because it aggregates local usage across developers
// rather than pointing at a deployed agent.
func devtoolIdentity(ad devtoolAdapter) InsightsIdentityRef {
	return InsightsIdentityRef{
		Kind:    "system",
		Label:   ad.Label,
		Icon:    ad.Icon,
		Tooltip: "Aggregated local dev-tool usage (" + ad.Label + ") across developers, not a deployed agent.",
	}
}

// devtoolAdapterByKey resolves a registered source key.
func devtoolAdapterByKey(key string) (devtoolAdapter, bool) {
	for _, ad := range devtoolAdapters {
		if ad.Key == key {
			return ad, true
		}
	}
	return devtoolAdapter{}, false
}
