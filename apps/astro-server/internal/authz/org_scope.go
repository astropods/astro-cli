package authz

// SessionOrgMatchesAccount reports whether the caller's JWT org scope matches an
// org-backed account. Personal accounts and accounts without a WorkOS org skip
// the check — same rules as RequireAccountPermission in middleware/account.go.
// FGAChecker uses this before sending a membership-scoped request to WorkOS.
func SessionOrgMatchesAccount(sub Subject, accountType, accountWorkOSOrgID string) bool {
	if accountType == "personal" || accountWorkOSOrgID == "" {
		return true
	}
	return sub.OrgID == accountWorkOSOrgID
}
