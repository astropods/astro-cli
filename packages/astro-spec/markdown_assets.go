package spec

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// MarkdownImage is a local (repo-relative) image reference discovered in an
// AGENT.md document.
type MarkdownImage struct {
	// Path is the cleaned, repo-relative path used both as the file to load and
	// as the key for rewriting. Percent-decoding, query/fragment stripping, and
	// lexical cleaning are already applied (e.g. "./docs/x.png?v=1" → "docs/x.png").
	Path string
}

// Markdown inline image: ![alt](url "optional title"). The URL token may be
// angle-bracket wrapped (<...>). Titles and surrounding whitespace are captured
// separately so a rewrite can replace only the URL.
var mdImageRe = regexp.MustCompile(`(!\[[^\]]*\]\(\s*)(<[^>]*>|[^)\s]+)([^)]*\))`)

// HTML <img ... src="url" ...>. The src value (with its quotes, if any) is
// captured separately from the surrounding tag.
var htmlImageRe = regexp.MustCompile(`(?i)(<img\b[^>]*?\bsrc\s*=\s*)("[^"]*"|'[^']*'|[^\s>]+)([^>]*>)`)

// ExtractMarkdownImages returns the deduplicated, first-seen-ordered set of
// local image references in md. Both Markdown image syntax and HTML <img> tags
// are scanned. Remote (scheme:// or //host), data:, anchor-only, root-absolute
// (/foo), and parent-escaping (../) references are skipped.
func ExtractMarkdownImages(md string) []MarkdownImage {
	var out []MarkdownImage
	seen := make(map[string]struct{})
	add := func(rawURL string) {
		if cleaned, ok := classifyLocalImagePath(rawURL); ok {
			if _, dup := seen[cleaned]; !dup {
				seen[cleaned] = struct{}{}
				out = append(out, MarkdownImage{Path: cleaned})
			}
		}
	}
	for _, m := range mdImageRe.FindAllStringSubmatch(md, -1) {
		add(m[2])
	}
	for _, m := range htmlImageRe.FindAllStringSubmatch(md, -1) {
		add(m[2])
	}
	return out
}

// RewriteMarkdownImages returns md with every local image reference whose
// cleaned path is a key in replace rewritten to the mapped URL. References not
// present in replace (including all remote references) are left untouched.
func RewriteMarkdownImages(md string, replace map[string]string) string {
	if len(replace) == 0 {
		return md
	}
	md = mdImageRe.ReplaceAllStringFunc(md, func(match string) string {
		sub := mdImageRe.FindStringSubmatch(match)
		newURL, ok := lookupReplacement(sub[2], replace)
		if !ok {
			return match
		}
		// Preserve any angle brackets around the original token.
		return sub[1] + wrapLikeOriginal(sub[2], newURL) + sub[3]
	})
	md = htmlImageRe.ReplaceAllStringFunc(md, func(match string) string {
		sub := htmlImageRe.FindStringSubmatch(match)
		newURL, ok := lookupReplacement(sub[2], replace)
		if !ok {
			return match
		}
		return sub[1] + requoteLikeOriginal(sub[2], newURL) + sub[3]
	})
	return md
}

// lookupReplacement classifies a raw URL token and returns its replacement URL
// when the token is a local reference present in replace.
func lookupReplacement(rawURL string, replace map[string]string) (string, bool) {
	cleaned, ok := classifyLocalImagePath(rawURL)
	if !ok {
		return "", false
	}
	u, ok := replace[cleaned]
	return u, ok
}

// wrapLikeOriginal re-wraps newURL in angle brackets when the original Markdown
// token was angle-bracket wrapped.
func wrapLikeOriginal(original, newURL string) string {
	if strings.HasPrefix(strings.TrimSpace(original), "<") {
		return "<" + newURL + ">"
	}
	return newURL
}

// requoteLikeOriginal re-applies the original HTML src quoting style to newURL.
func requoteLikeOriginal(original, newURL string) string {
	switch {
	case strings.HasPrefix(original, `"`):
		return `"` + newURL + `"`
	case strings.HasPrefix(original, `'`):
		return `'` + newURL + `'`
	default:
		return newURL
	}
}

// classifyLocalImagePath normalizes a raw image reference and reports whether it
// is a repo-local path that should be vacuumed. Returns the cleaned path and
// true for local references; "", false otherwise.
func classifyLocalImagePath(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		s = s[1 : len(s)-1]
	}
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	// Drop query and fragment suffixes (?v=1, #anchor).
	if i := strings.IndexAny(s, "#?"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "", false
	}
	// Remote, data:, mailto:, protocol-relative — not local.
	if hasURLScheme(s) || strings.HasPrefix(s, "//") {
		return "", false
	}
	// Root-absolute paths (/foo) are repo-root relative on GitHub but agent-dir
	// relative on push; the semantics diverge, so they are out of scope.
	if strings.HasPrefix(s, "/") {
		return "", false
	}
	if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	cleaned := path.Clean(s)
	if cleaned == "." || cleaned == "" {
		return "", false
	}
	// Reject paths that escape the repo root.
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// hasURLScheme reports whether s begins with a URI scheme (e.g. "https:",
// "data:"). Follows RFC 3986: ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) ":".
func hasURLScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ':':
			return i > 0
		case i == 0:
			if !isASCIILetter(c) {
				return false
			}
		default:
			if !isASCIILetter(c) && !isASCIIDigit(c) && c != '+' && c != '-' && c != '.' {
				return false
			}
		}
	}
	return false
}

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isASCIIDigit(c byte) bool  { return c >= '0' && c <= '9' }
