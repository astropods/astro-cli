// Package readmeassets stores images referenced by an agent's AGENT.md in the
// shared assets bucket and produces their public CDN URLs. It reuses the avatar
// storage backend (same bucket + CDN) but, unlike avatars, stores images as-is
// (no resize/re-encode) under content-addressed keys so identical images are
// stored once and are safe to cache indefinitely.
package readmeassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
)

const (
	// MaxAssetSize is the largest single readme image accepted (10 MB).
	MaxAssetSize = 10 << 20
	// MaxAssets is the most images vacuumed from a single AGENT.md.
	MaxAssets = 20
	// keyPrefix namespaces readme images within the shared assets bucket.
	keyPrefix = "readme-assets"
)

// Backend is the subset of object-store operations readme assets require. It is
// satisfied by *avatar.S3Backend and *avatar.LocalBackend (the same bucket the
// avatar store writes to).
type Backend interface {
	Write(ctx context.Context, key string, data []byte, contentType string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// Store uploads AGENT.md images and resolves their public URLs.
type Store struct {
	backend   Backend
	assetsURL string // CDN base URL, e.g. "https://assets.astropods.ai"
}

// NewStore creates a store over the given backend. When assetsURL is empty
// (local dev) it defaults to "/assets", matching the avatar store so URLs
// resolve against the on-disk assets directory served at /assets.
func NewStore(backend Backend, assetsURL string) *Store {
	if assetsURL == "" {
		assetsURL = "/assets"
	}
	return &Store{backend: backend, assetsURL: assetsURL}
}

// Upload stores a single image for the given account/agent and returns its
// public URL. Identical bytes resolve to the same content-addressed key and are
// not re-uploaded. Returns an error if the data exceeds MaxAssetSize or is not a
// supported image type.
func (s *Store) Upload(ctx context.Context, account, name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty image")
	}
	if len(data) > MaxAssetSize {
		return "", fmt.Errorf("image too large: %d bytes (max %d)", len(data), MaxAssetSize)
	}
	contentType, ext, ok := detectImageType(data)
	if !ok {
		return "", fmt.Errorf("unsupported image type")
	}

	key := assetKey(account, name, data, ext)
	exists, err := s.backend.Exists(ctx, key)
	if err != nil {
		return "", err
	}
	if !exists {
		if err := s.backend.Write(ctx, key, data, contentType); err != nil {
			return "", err
		}
	}
	return s.assetsURL + "/" + key, nil
}

// assetKey returns the content-addressed storage key for an image.
func assetKey(account, name string, data []byte, ext string) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s/%s/%s/%s%s", keyPrefix, account, name, hex.EncodeToString(sum[:]), ext)
}

// detectImageType returns the content type and file extension for supported
// image bytes. SVG is sniffed separately because http.DetectContentType reports
// it as text. Returns ok=false for unsupported types.
func detectImageType(data []byte) (contentType, ext string, ok bool) {
	if avatar.IsSVGContent(data) {
		return "image/svg+xml", ".svg", true
	}
	switch http.DetectContentType(data) {
	case "image/png":
		return "image/png", ".png", true
	case "image/jpeg":
		return "image/jpeg", ".jpg", true
	case "image/gif":
		return "image/gif", ".gif", true
	case "image/webp":
		return "image/webp", ".webp", true
	}
	return "", "", false
}
