package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

// pushLinks is the local, project-scoped record of where `ast push` last
// resolved for each blueprint name pushed from this project. Stored at
// .ast/push.json, next to the project's spec file: a project pushed under
// several names (e.g. staging/prod) keeps a separate, independent link
// for each, and a project selected via `-f` links relative to the spec's
// own directory, not whatever directory the command happened to run from.
// Machine-local state, not shared project config: scaffolded projects
// gitignore .ast/ (internal/scaffold/templates/*/gitignore.tmpl) so this
// is never committed for them, and it shouldn't be for any other project
// either, since a committed link would make a collaborator's first push
// of a name skip the confirmation this file exists to gate.
type pushLinks struct {
	Links map[string]string `json:"links"`
}

func pushLinkPath(specDir string) string {
	return filepath.Join(specDir, buildinfo.AppDirName, "push.json")
}

// readPushLinks returns the stored links for specDir, or a zero value if
// none exist, or the file can't be read or parsed: a corrupt or
// unreadable local cache file should never block a push.
func readPushLinks(specDir string) pushLinks {
	data, err := os.ReadFile(pushLinkPath(specDir)) //nolint:gosec // path is specDir + hardcoded suffix
	if err != nil {
		return pushLinks{}
	}
	var links pushLinks
	if err := json.Unmarshal(data, &links); err != nil {
		return pushLinks{}
	}
	return links
}

// account returns the account name linked to name, and whether one was found.
func (l pushLinks) account(name string) (string, bool) {
	account, ok := l.Links[name]
	return account, ok
}

// writePushLink records account as name's push target for specDir,
// alongside any other names already linked there.
func writePushLink(specDir, name, account string) error {
	links := readPushLinks(specDir)
	if links.Links == nil {
		links.Links = map[string]string{}
	}
	links.Links[name] = account

	path := pushLinkPath(specDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec
		return err
	}
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644) //nolint:gosec
}
