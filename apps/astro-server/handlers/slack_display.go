package handlers

import (
	"strings"
	"unicode"

	slackclient "github.com/astropods/astro/apps/astro-server/internal/slack"
)

func slackObservedDisplayName(user slackclient.UserInfo) string {
	if name := strings.TrimSpace(user.RealName); name != "" {
		return name
	}
	if name := strings.TrimSpace(user.DisplayName); name != "" {
		return name
	}
	return strings.TrimSpace(user.Name)
}

func slackDisplayNameFromProfile(displayName, username string) string {
	displayName = strings.TrimSpace(displayName)
	username = strings.TrimSpace(username)
	if displayName == "" {
		return slackDisplayNameFromUsername(username)
	}
	if isStaleSlackHandleDisplayName(displayName, username) {
		return slackDisplayNameFromUsername(username)
	}
	return displayName
}

func isStaleSlackHandleDisplayName(displayName, username string) bool {
	if displayName == "" || username == "" || displayName != username || displayName != strings.ToLower(displayName) {
		return false
	}
	parts := strings.FieldsFunc(displayName, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len([]rune(part)) <= 1 {
			return false
		}
	}
	return true
}

func slackDisplayNameFromUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	parts := strings.FieldsFunc(username, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, titleSlackNamePart(part))
	}
	if len(out) == 0 {
		return username
	}
	return strings.Join(out, " ")
}

func titleSlackNamePart(part string) string {
	runes := []rune(strings.ToLower(part))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
