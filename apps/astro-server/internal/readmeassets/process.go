package readmeassets

import (
	"context"
	"fmt"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// ProcessMarkdown vacuums the local images referenced by readme: each is fetched
// via fetch, uploaded, and its reference rewritten to the resulting CDN URL. The
// rewritten markdown is returned along with warnings for images that could not
// be fetched or stored — those are left as their original reference rather than
// failing the build. fetch receives the cleaned repo-relative path and returns
// the image bytes (empty bytes are treated as "not found").
//
// This is the server-side path (GitHub builds), where the server fetches images
// itself. The CLI push path instead uploads bytes via Upload and rewrites with
// spec.RewriteMarkdownImages at registration time.
func (s *Store) ProcessMarkdown(
	ctx context.Context,
	account, name, readme string,
	fetch func(relPath string) ([]byte, error),
) (string, []string) {
	images := spec.ExtractMarkdownImages(readme)
	if len(images) == 0 {
		return readme, nil
	}

	var warnings []string
	if len(images) > MaxAssets {
		warnings = append(warnings, fmt.Sprintf("AGENT.md references %d local images; only the first %d are stored", len(images), MaxAssets))
		images = images[:MaxAssets]
	}

	replace := make(map[string]string, len(images))
	for _, img := range images {
		data, err := fetch(img.Path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("fetch %q: %v", img.Path, err))
			continue
		}
		if len(data) == 0 {
			warnings = append(warnings, fmt.Sprintf("image %q not found in repo", img.Path))
			continue
		}
		url, err := s.Upload(ctx, account, name, data)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("store %q: %v", img.Path, err))
			continue
		}
		replace[img.Path] = url
	}

	return spec.RewriteMarkdownImages(readme, replace), warnings
}
