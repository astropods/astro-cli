package account

import (
	"fmt"
	"strings"
	"unicode"
)

// reserved names that cannot be used as account names
var reservedNames = map[string]bool{
	// Current frontend routes
	"admin":         true,
	"agents":        true,
	"blueprints":    true,
	"dashboard":     true,
	"deploy":        true,
	"onboarding":    true,
	"organization":  true,
	"request-agent": true,
	"settings":      true,

	// Current backend / infra routes
	"api":      true,
	"auth":     true,
	"health":   true,
	"ready":    true,
	"schema":   true,
	"status":   true,
	"undeploy": true,

	// Auth & identity
	"activate":  true,
	"anonymous": true,
	"callback":  true,
	"confirm":   true,
	"identity":  true,
	"invite":    true,
	"invites":   true,
	"join":      true,
	"login":     true,
	"logout":    true,
	"oauth":     true,
	"register":  true,
	"sessions":  true,
	"signin":    true,
	"signup":    true,
	"username":  true,
	"verify":    true,

	// User / account variants
	"account":       true,
	"accounts":      true,
	"home":          true,
	"individual":    true,
	"me":            true,
	"members":       true,
	"organizations": true,
	"orgs":          true,
	"personal":      true,
	"profile":       true,
	"team":          true,
	"teams":         true,
	"user":          true,
	"users":         true,

	// Product features (current + likely future)
	"analytics":     true,
	"apps":          true,
	"billing":       true,
	"catalog":       true,
	"checkout":      true,
	"cloud":         true,
	"deployments":   true,
	"design":        true,
	"discover":      true,
	"editor":        true,
	"editors":       true,
	"explore":       true,
	"features":      true,
	"images":        true,
	"integrations":  true,
	"invoices":      true,
	"jobs":          true,
	"knowledge":     true,
	"library":       true,
	"listings":      true,
	"lists":         true,
	"logs":          true,
	"marketplace":   true,
	"metrics":       true,
	"mobile":        true,
	"models":        true,
	"monitoring":    true,
	"observability": true,
	"payments":      true,
	"pipelines":     true,
	"plans":         true,
	"plugins":       true,
	"pricing":       true,
	"releases":      true,
	"runs":          true,
	"services":      true,
	"solutions":     true,
	"stories":       true,
	"subscriptions": true,
	"tasks":         true,
	"templates":     true,
	"topic":         true,
	"topics":        true,
	"traces":        true,
	"usage":         true,
	"workflows":     true,

	// Content & docs
	"about":         true,
	"blog":          true,
	"changelog":     true,
	"contact":       true,
	"cookies":       true,
	"docs":          true,
	"documentation": true,
	"guide":         true,
	"guides":        true,
	"help":          true,
	"info":          true,
	"journal":       true,
	"journals":      true,
	"learn":         true,
	"legal":         true,
	"news":          true,
	"newsletter":    true,
	"newsroom":      true,
	"pages":         true,
	"posts":         true,
	"privacy":       true,
	"readme":        true,
	"resources":     true,
	"security":      true,
	"site":          true,
	"sitemap":       true,
	"talks":         true,
	"terms":         true,
	"timeline":      true,
	"translations":  true,
	"trending":      true,
	"updates":       true,
	"wiki":          true,
	"wikis":         true,

	// Developer
	"console":    true,
	"developer":  true,
	"developers": true,
	"garage":     true,
	"graphql":    true,
	"lab":        true,
	"labs":       true,
	"platform":   true,
	"playground": true,
	"sandbox":    true,
	"ssh":        true,
	"webhook":    true,
	"webhooks":   true,

	// Marketing & business
	"alpha":            true,
	"beta":             true,
	"builders":         true,
	"business":         true,
	"businesses":       true,
	"careers":          true,
	"case-studies":     true,
	"customer-stories": true,
	"customers":        true,
	"demo":             true,
	"enterprise":       true,
	"investors":        true,
	"launch":           true,
	"nonprofits":       true,
	"offer":            true,
	"partners":         true,
	"popular":          true,
	"press":            true,
	"preview":          true,
	"professional":     true,
	"referral":         true,
	"shareholders":     true,
	"showcases":        true,
	"site-policy":      true,
	"social-impact":    true,
	"sponsors":         true,
	"staff":            true,
	"store":            true,
	"tour":             true,
	"trial":            true,

	// Safety & abuse
	"abuse":                 true,
	"invalid-email-address": true,
	"malware":               true,
	"spam":                  true,
	"suspended":             true,

	// Common URL verbs / actions
	"browse":      true,
	"configure":   true,
	"connect":     true,
	"create":      true,
	"delete":      true,
	"edit":        true,
	"export":      true,
	"import":      true,
	"manage":      true,
	"migrate":     true,
	"remove":      true,
	"search":      true,
	"suggest":     true,
	"suggestion":  true,
	"suggestions": true,
	"train":       true,
	"training":    true,
	"transfer":    true,
	"watching":    true,

	// Infra & internal
	"assets":    true,
	"download":  true,
	"downloads": true,
	"downtime":  true,
	"files":     true,
	"internal":  true,
	"linux":     true,
	"mac":       true,
	"new":       true,
	"operator":  true,
	"public":    true,
	"registry":  true,
	"repo":      true,
	"repos":     true,
	"static":    true,
	"support":   true,
	"system":    true,
	"uploads":   true,
	"windows":   true,
	"www":       true,

	// Workspace / project
	"activity":      true,
	"audit":         true,
	"chat":          true,
	"dev":           true,
	"environments":  true,
	"errors":        true,
	"events":        true,
	"feed":          true,
	"hire":          true,
	"inbox":         true,
	"maintenance":   true,
	"messages":      true,
	"notifications": true,
	"project":       true,
	"projects":      true,
	"workspace":     true,
	"workspaces":    true,
}

// ValidateAccountName validates an account name format:
// - 4-39 characters
// - lowercase alphanumeric + hyphens only
// - must start with a letter
// - must not end with a hyphen
// - no consecutive hyphens
func ValidateAccountName(name string) error {
	if len(name) < 4 {
		return fmt.Errorf("account name must be at least 4 characters")
	}
	if len(name) > 39 {
		return fmt.Errorf("account name must be at most 39 characters")
	}

	// Must be lowercase
	if name != strings.ToLower(name) {
		return fmt.Errorf("account name must be lowercase")
	}

	// Must start with a letter
	if !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("account name must start with a letter")
	}

	// Must not end with a hyphen
	if name[len(name)-1] == '-' {
		return fmt.Errorf("account name must not end with a hyphen")
	}

	// Check each character and no consecutive hyphens
	prevHyphen := false
	for _, ch := range name {
		if ch == '-' {
			if prevHyphen {
				return fmt.Errorf("account name must not contain consecutive hyphens")
			}
			prevHyphen = true
			continue
		}
		prevHyphen = false
		if !unicode.IsLower(ch) && !unicode.IsDigit(ch) {
			return fmt.Errorf("account name must contain only lowercase letters, digits, and hyphens")
		}
	}

	return nil
}

// CheckAccountNameRestricted checks if a name is reserved or denied.
// Call this in addition to ValidateAccountName for user-facing registration
// where brand/system names should be blocked.
func CheckAccountNameRestricted(name string) error {
	if reservedNames[name] || reservedVariants[name] {
		return fmt.Errorf("account name %q is reserved", name)
	}
	if deniedNames[name] {
		return fmt.Errorf("account name %q is reserved for brand use — contact support to request it", name)
	}
	return nil
}
