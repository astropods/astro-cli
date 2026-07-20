package handlers

import (
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// Identity hydration is the read-time layer that fills in
// display_name / username / avatar_url onto a user row based on its
// `kind`. Two sources feed it:
//
//   - Slack-kind rows resolve through the slackidentity directory tables
//     for team_id + Slack profile + bot/deleted flags.
//   - Astro-kind rows resolve through the per-user personal-account row
//     (account.GetPersonalProfiles) for display_name + username slug.
//
// Both lookups are deduplicated and batched per resolver call.

// userDetailsHydrator pre-fetches the directory + profile data for a
// batch of user_ids and exposes a stamp() that applies it row by row.
type userDetailsHydrator struct {
	slack map[string]slackidentity.DirectoryEntry
	astro map[string]account.PersonalProfile
}

func newUserDetailsHydrator(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	accountStore *account.AccountStore,
	userIDs []string,
	contextLabel string,
) *userDetailsHydrator {
	h := &userDetailsHydrator{
		slack: map[string]slackidentity.DirectoryEntry{},
		astro: map[string]account.PersonalProfile{},
	}
	var slackIDs, astroIDs []string
	seen := make(map[string]struct{}, len(userIDs))
	for _, uid := range userIDs {
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		switch classifyUserID(uid) {
		case UserDetailsKindSlack:
			slackIDs = append(slackIDs, uid)
		case UserDetailsKindAstro:
			astroIDs = append(astroIDs, uid)
		}
	}
	if slackStore != nil && len(slackIDs) > 0 {
		entries, err := slackStore.DirectoryEntriesForSlackUserIDs(slackIDs)
		if err != nil {
			log.Warn(contextLabel+": slack directory lookup failed; rows render with id only", "error", err)
		} else {
			h.slack = entries
		}
	}
	if accountStore != nil && len(astroIDs) > 0 {
		profiles, err := accountStore.GetPersonalProfiles(astroIDs)
		if err != nil {
			log.Warn(contextLabel+": astro personal-profile lookup failed; rows render with id only", "error", err)
		} else {
			h.astro = profiles
		}
	}
	return h
}

// stamp applies hydrated identity data onto details in place. No-op for
// kind="unknown" or for known kinds with no lookup hit.
//
// When details.Kind is empty — the case for cache entries written by a
// pre-discriminated-union build of the server — we infer it from the
// user_id shape via classifyUserID. Without this backfill, every old
// cached row would render as "Unknown user" for up to one refresh cycle
// (6h) after deploy because the switch below wouldn't match any case.
func (h *userDetailsHydrator) stamp(userID string, details *UserDetails) {
	if details.Kind == "" {
		details.Kind = classifyUserID(userID)
	}
	switch details.Kind {
	case UserDetailsKindSlack:
		if entry, ok := h.slack[userID]; ok {
			*details = userDetailsFromEntry(userID, &entry)
		}
	case UserDetailsKindAstro:
		if profile, ok := h.astro[userID]; ok {
			details.DisplayName = profile.DisplayName
			details.Username = profile.Name
		}
	}
}

// ResolveUsersSummaryIdentities stamps Slack + Astro profile metadata
// onto the users-summary response in place.
func ResolveUsersSummaryIdentities(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	accountStore *account.AccountStore,
	resp *AccountUsersSummaryResponse,
) {
	if resp == nil || len(resp.Users) == 0 {
		return
	}
	ids := make([]string, 0, len(resp.Users))
	for _, u := range resp.Users {
		ids = append(ids, u.UserID)
	}
	h := newUserDetailsHydrator(log, slackStore, accountStore, ids, "users-summary")
	for i := range resp.Users {
		h.stamp(resp.Users[i].UserID, &resp.Users[i].UserDetails)
	}
}

// ResolveAccountSummaryIdentities stamps team + profile metadata onto
// per-day per-user cost entries in CostOverTimeByUser. Bucketing already
// happened upstream via translation, so this is a pure in-place stamp.
func ResolveAccountSummaryIdentities(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	accountStore *account.AccountStore,
	resp *AccountObservabilitySummaryResponse,
) {
	if resp == nil || len(resp.CostOverTimeByUser) == 0 {
		return
	}
	idSet := map[string]struct{}{}
	for _, entry := range resp.CostOverTimeByUser {
		for _, u := range entry.Users {
			idSet[u.UserID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	h := newUserDetailsHydrator(log, slackStore, accountStore, ids, "account-summary cost-over-time")
	for di := range resp.CostOverTimeByUser {
		users := resp.CostOverTimeByUser[di].Users
		for ui := range users {
			h.stamp(users[ui].UserID, &users[ui].UserDetails)
		}
	}
}

// ResolveDeploymentsSummaryIdentities stamps Slack + Astro profile data
// onto each deployment's UsersUsedDetails entries.
func ResolveDeploymentsSummaryIdentities(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	accountStore *account.AccountStore,
	resp *AccountDeploymentsSummaryResponse,
) {
	if resp == nil || len(resp.Deployments) == 0 {
		return
	}
	idSet := map[string]struct{}{}
	for _, dep := range resp.Deployments {
		for _, u := range dep.UsersUsedDetails {
			idSet[u.UserID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	h := newUserDetailsHydrator(log, slackStore, accountStore, ids, "deployments-summary")
	for di := range resp.Deployments {
		details := resp.Deployments[di].UsersUsedDetails
		for ui := range details {
			h.stamp(details[ui].UserID, &details[ui].UserDetails)
		}
	}
}
