package spec

import (
	"reflect"
	"testing"
)

func TestExtractMarkdownImages(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "relative markdown image",
			md:   "text\n![diagram](./docs/arch.png)\nmore",
			want: []string{"docs/arch.png"},
		},
		{
			name: "bare relative path",
			md:   "![x](images/logo.svg)",
			want: []string{"images/logo.svg"},
		},
		{
			name: "image with title and query",
			md:   `![x](assets/a.png?v=2 "A title")`,
			want: []string{"assets/a.png"},
		},
		{
			name: "angle-bracket url",
			md:   "![x](<docs/my image.png>)",
			want: []string{"docs/my image.png"},
		},
		{
			name: "percent-encoded space",
			md:   "![x](docs/my%20image.png)",
			want: []string{"docs/my image.png"},
		},
		{
			name: "html img double quotes",
			md:   `<img src="screenshots/home.png" alt="home">`,
			want: []string{"screenshots/home.png"},
		},
		{
			name: "html img single quotes and other attrs first",
			md:   `<img width="40" src='icons/star.png' />`,
			want: []string{"icons/star.png"},
		},
		{
			name: "remote and data urls skipped",
			md:   "![a](https://x.com/a.png) ![b](http://x/b.png) ![c](data:image/png;base64,AAAA) ![d](//cdn/x.png)",
			want: nil,
		},
		{
			name: "root-absolute and parent-escape skipped",
			md:   "![a](/docs/a.png) ![b](../../etc/passwd)",
			want: nil,
		},
		{
			name: "dedup keeps first-seen order",
			md:   "![a](docs/a.png) ![b](b.png) ![a2](./docs/a.png)",
			want: []string{"docs/a.png", "b.png"},
		},
		{
			name: "anchor only skipped",
			md:   "![a](#section)",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMarkdownImages(tt.md)
			var paths []string
			for _, g := range got {
				paths = append(paths, g.Path)
			}
			if !reflect.DeepEqual(paths, tt.want) {
				t.Errorf("ExtractMarkdownImages() = %v, want %v", paths, tt.want)
			}
		})
	}
}

func TestRewriteMarkdownImages(t *testing.T) {
	replace := map[string]string{
		"docs/arch.png":     "https://assets.example/readme-assets/acc/agent/h1.png",
		"docs/my image.png": "https://assets.example/readme-assets/acc/agent/h2.png",
		"icons/star.png":    "https://assets.example/readme-assets/acc/agent/h3.png",
	}
	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "rewrites relative markdown image, preserves title",
			md:   `![diagram](./docs/arch.png "Architecture")`,
			want: `![diagram](https://assets.example/readme-assets/acc/agent/h1.png "Architecture")`,
		},
		{
			name: "rewrites angle-bracket url and drops brackets-preserving form",
			md:   "![x](<docs/my image.png>)",
			want: "![x](<https://assets.example/readme-assets/acc/agent/h2.png>)",
		},
		{
			name: "rewrites html img preserving quote style and attrs",
			md:   `<img width="40" src='icons/star.png' />`,
			want: `<img width="40" src='https://assets.example/readme-assets/acc/agent/h3.png' />`,
		},
		{
			name: "leaves remote and unmapped untouched",
			md:   "![a](https://x/a.png) ![b](docs/other.png)",
			want: "![a](https://x/a.png) ![b](docs/other.png)",
		},
		{
			name: "rewrites query-suffixed reference to clean target",
			md:   "![a](docs/arch.png?v=9)",
			want: "![a](https://assets.example/readme-assets/acc/agent/h1.png)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteMarkdownImages(tt.md, replace)
			if got != tt.want {
				t.Errorf("RewriteMarkdownImages()\n got = %q\nwant = %q", got, tt.want)
			}
		})
	}
}
