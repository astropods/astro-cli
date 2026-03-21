package avatar

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoder
)

const (
	// MaxUploadSize is the maximum allowed avatar upload size (5 MB).
	MaxUploadSize = 5 << 20
	// OutputSize is the square dimension of processed avatars.
	OutputSize = 512
	// PresetCount is the number of preset placeholder avatars.
	PresetCount = 25
	// jpegQuality is the JPEG encoding quality for processed avatars.
	jpegQuality = 85
)

// Backend abstracts the storage layer for avatar images.
// Implemented by S3Backend and LocalBackend.
type Backend interface {
	// Read returns the contents of a file at the given key.
	Read(ctx context.Context, key string) ([]byte, error)
	// Write stores data at the given key with the specified content type.
	Write(ctx context.Context, key string, data []byte, contentType string) error
	// Copy duplicates a file from src to dst.
	Copy(ctx context.Context, src, dst string) error
	// Delete removes a file at the given key.
	Delete(ctx context.Context, key string) error
	// Exists returns true if a file exists at the given key.
	Exists(ctx context.Context, key string) (bool, error)
}

// Store manages avatar images using a pluggable storage backend.
type Store struct {
	backend   Backend
	assetsURL string // CDN base URL, e.g. "https://assets.astropods.ai"
}

// NewStore creates a new avatar store with the given backend.
func NewStore(backend Backend, assetsURL string) *Store {
	return &Store{
		backend:   backend,
		assetsURL: assetsURL,
	}
}

// AvatarURL returns the CDN URL for an account's avatar with cache-busting version.
func (s *Store) AvatarURL(handle string, version int) string {
	return fmt.Sprintf("%s/avatars/%s.jpg?v=%d", s.assetsURL, handle, version)
}

// PresetIndex returns the deterministic preset index (1-25) for a handle.
func PresetIndex(handle string) int {
	hash := uint32(0)
	for _, c := range handle {
		hash = hash*31 + uint32(c)
	}
	return int(hash%PresetCount) + 1
}

// presetKey returns the storage key for a preset avatar SVG.
func presetKey(index int) string {
	return fmt.Sprintf("placeholders/accounts/avatar_%02d.svg", index)
}

// avatarKey returns the storage key for an account's avatar.
func avatarKey(handle string) string {
	return fmt.Sprintf("avatars/%s.jpg", handle)
}

// AvatarExists checks whether an avatar exists for the given handle.
func (s *Store) AvatarExists(ctx context.Context, handle string) (bool, error) {
	return s.backend.Exists(ctx, avatarKey(handle))
}

// AssignPreset copies a deterministic preset placeholder to the account's avatar key.
func (s *Store) AssignPreset(ctx context.Context, handle string) error {
	return s.SetPreset(ctx, handle, PresetIndex(handle))
}

// SetPreset copies a specific preset (1-25) to the account's avatar key.
func (s *Store) SetPreset(ctx context.Context, handle string, index int) error {
	if index < 1 || index > PresetCount {
		return fmt.Errorf("preset index must be 1-%d, got %d", PresetCount, index)
	}
	return s.backend.Copy(ctx, presetKey(index), avatarKey(handle))
}

// Upload validates, resizes, and stores an avatar image for the given handle.
func (s *Store) Upload(ctx context.Context, handle string, imageBytes []byte) error {
	if len(imageBytes) > MaxUploadSize {
		return fmt.Errorf("image too large: %d bytes (max %d)", len(imageBytes), MaxUploadSize)
	}

	contentType := http.DetectContentType(imageBytes)
	if !isAllowedImageType(contentType) {
		return fmt.Errorf("unsupported image type: %s", contentType)
	}

	src, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, OutputSize, OutputSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}

	return s.backend.Write(ctx, avatarKey(handle), buf.Bytes(), "image/jpeg")
}

// Ingest fetches an image from an external URL and uploads it as the account's avatar.
func (s *Store) Ingest(ctx context.Context, handle string, externalURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, externalURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch external image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch external image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxUploadSize+1))
	if err != nil {
		return fmt.Errorf("read external image: %w", err)
	}
	if len(data) > MaxUploadSize {
		return fmt.Errorf("external image too large: > %d bytes", MaxUploadSize)
	}

	return s.Upload(ctx, handle, data)
}

// Move copies an avatar from oldHandle to newHandle and deletes the old key.
func (s *Store) Move(ctx context.Context, oldHandle, newHandle string) error {
	if err := s.backend.Copy(ctx, avatarKey(oldHandle), avatarKey(newHandle)); err != nil {
		return fmt.Errorf("copy avatar %s -> %s: %w", oldHandle, newHandle, err)
	}
	if err := s.backend.Delete(ctx, avatarKey(oldHandle)); err != nil {
		return fmt.Errorf("delete old avatar %s: %w", oldHandle, err)
	}
	return nil
}

// Delete removes an account's avatar.
func (s *Store) Delete(ctx context.Context, handle string) error {
	return s.backend.Delete(ctx, avatarKey(handle))
}

func isAllowedImageType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return true
	}
	return false
}
