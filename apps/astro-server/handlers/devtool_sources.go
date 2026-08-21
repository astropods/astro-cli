package handlers

import (
	"net/url"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
)

// Dev-tool spend (Claude Code, …) reaches Insights through the same fact table
// as agent spend, so a tool needs no read path of its own. This registry is what
// names and brands it: one entry gives it a Sources-filter option, a synthetic
// agents-table row, and chips on the People rows.

// devtoolAdapter registers one coding tool. Key is both the astro.source value
// and the Langfuse trace tag.
type devtoolAdapter struct {
	Key   string
	Label string
	Icon  string // integration-icon key → themed logo (e.g. "claude-code")
}

var devtoolAdapters = []devtoolAdapter{
	{Key: "claude-code", Label: "Claude Code", Icon: "claude-code"},
}

// devtoolIdentity is the row identity for a dev-tool source: system-kind, brand
// icon, and a link to the source's detail page rather than to a deployment.
//
// Absolute and account-stamped: the table only routes an href beginning with
// "/" client-side, and a plain anchor reloads the page, dropping the scope.
func devtoolIdentity(ad devtoolAdapter, accountName string, linkToDetail bool) InsightsIdentityRef {
	ref := InsightsIdentityRef{
		Kind:    "system",
		Label:   ad.Label,
		Icon:    ad.Icon,
		Tooltip: "Aggregated local dev-tool usage (" + ad.Label + ") across developers, not a deployed agent.",
	}
	if linkToDetail {
		ref.Href = "/insights/sources/" + ad.Key + "?account=" + url.QueryEscape(accountName)
	}
	return ref
}

// devtoolActorFor maps a dev-tool email onto the actor key space
// insights_usage_daily uses, which is what lets dev-tool and agent spend merge
// into one People row without a special case. Both fact tables depend on this
// rule agreeing, so it has one definition.
func devtoolActorFor(email string, emailToUserID map[string]string) (kind, key string) {
	if email == "" {
		return insightsrollup.ActorKindUnidentified, ""
	}
	if uid, ok := emailToUserID[strings.ToLower(email)]; ok && uid != "" {
		return insightsrollup.ActorKindMember, uid
	}
	return insightsrollup.ActorKindUnidentified, email
}

func devtoolTagged(rawTags any) bool {
	for _, ad := range devtoolAdapters {
		if hasTag(rawTags, ad.Key) {
			return true
		}
	}
	return false
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
