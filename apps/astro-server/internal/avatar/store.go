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
// When assetsURL is empty (local dev), defaults to "/assets" so server-computed
// avatar URLs resolve against the Vite dev server's public directory.
func NewStore(backend Backend, assetsURL string) *Store {
	if assetsURL == "" {
		assetsURL = "/assets"
	}
	return &Store{
		backend:   backend,
		assetsURL: assetsURL,
	}
}

// AvatarURL returns the CDN URL for an account's avatar.
func (s *Store) AvatarURL(handle string) string {
	return fmt.Sprintf("%s/avatars/%s.jpg", s.assetsURL, handle)
}

// PresetIndex returns the deterministic preset index (1-25) for a handle.
func PresetIndex(handle string) int {
	hash := uint32(0)
	for _, c := range handle {
		hash = hash*31 + uint32(c) //nolint:gosec // intentional truncation for hash
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

// agentAvatarKey returns the storage key for an agent blueprint's avatar.
func agentAvatarKey(account, name string) string {
	return fmt.Sprintf("avatars/agents/%s/%s.jpg", account, name)
}

// deploymentAvatarKey returns the storage key for a deployment's avatar.
func deploymentAvatarKey(id string) string {
	return fmt.Sprintf("avatars/deployments/%s.jpg", id)
}

// AvatarExists checks whether an avatar exists for the given handle.
func (s *Store) AvatarExists(ctx context.Context, handle string) (bool, error) {
	return s.backend.Exists(ctx, avatarKey(handle))
}

// AgentAvatarExists checks whether an avatar exists for the given agent.
func (s *Store) AgentAvatarExists(ctx context.Context, account, name string) (bool, error) {
	return s.backend.Exists(ctx, agentAvatarKey(account, name))
}

// DeploymentAvatarExists checks whether an avatar exists for the given deployment.
func (s *Store) DeploymentAvatarExists(ctx context.Context, id string) (bool, error) {
	return s.backend.Exists(ctx, deploymentAvatarKey(id))
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

// processImage validates, decodes, resizes to 512x512, and JPEG-encodes an image.
func processImage(imageBytes []byte) ([]byte, error) {
	if len(imageBytes) > MaxUploadSize {
		return nil, fmt.Errorf("image too large: %d bytes (max %d)", len(imageBytes), MaxUploadSize)
	}

	contentType := http.DetectContentType(imageBytes)
	if !isAllowedImageType(contentType) {
		return nil, fmt.Errorf("unsupported image type: %s", contentType)
	}

	src, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, OutputSize, OutputSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return buf.Bytes(), nil
}

// uploadToKey processes an image and writes it to the given storage key.
func (s *Store) uploadToKey(ctx context.Context, key string, imageBytes []byte) error {
	data, err := processImage(imageBytes)
	if err != nil {
		return err
	}
	return s.backend.Write(ctx, key, data, "image/jpeg")
}

// Upload validates, resizes, and stores an avatar image for the given account handle.
func (s *Store) Upload(ctx context.Context, handle string, imageBytes []byte) error {
	return s.uploadToKey(ctx, avatarKey(handle), imageBytes)
}

// UploadAgent validates, resizes, and stores an avatar for an agent blueprint.
func (s *Store) UploadAgent(ctx context.Context, account, name string, imageBytes []byte) error {
	return s.uploadToKey(ctx, agentAvatarKey(account, name), imageBytes)
}

// DeleteAgent removes an agent blueprint's avatar.
func (s *Store) DeleteAgent(ctx context.Context, account, name string) error {
	return s.backend.Delete(ctx, agentAvatarKey(account, name))
}

// AgentAvatarURL returns the CDN URL for an agent blueprint's avatar.
func (s *Store) AgentAvatarURL(account, name string) string {
	return fmt.Sprintf("%s/%s", s.assetsURL, agentAvatarKey(account, name))
}

// CopyAgentToDeployment copies a blueprint's avatar to a deployment.
// Returns true if a copy was made, false if the blueprint had no avatar.
func (s *Store) CopyAgentToDeployment(ctx context.Context, account, agentName, deploymentID string) (bool, error) {
	src := agentAvatarKey(account, agentName)
	exists, err := s.backend.Exists(ctx, src)
	if err != nil {
		return false, fmt.Errorf("check agent avatar: %w", err)
	}
	if !exists {
		return false, nil
	}
	dst := deploymentAvatarKey(deploymentID)
	if err := s.backend.Copy(ctx, src, dst); err != nil {
		return false, fmt.Errorf("copy agent avatar to deployment: %w", err)
	}
	return true, nil
}

// UploadDeployment validates, resizes, and stores an avatar for a deployment.
func (s *Store) UploadDeployment(ctx context.Context, id string, imageBytes []byte) error {
	return s.uploadToKey(ctx, deploymentAvatarKey(id), imageBytes)
}

// DeleteDeployment removes a deployment's avatar.
func (s *Store) DeleteDeployment(ctx context.Context, id string) error {
	return s.backend.Delete(ctx, deploymentAvatarKey(id))
}

// DeploymentAvatarURL returns the CDN URL for a deployment's avatar.
func (s *Store) DeploymentAvatarURL(id string) string {
	return fmt.Sprintf("%s/%s", s.assetsURL, deploymentAvatarKey(id))
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
	defer func() { _ = resp.Body.Close() }()

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

// MoveAgentAvatar moves an agent avatar from one account/name path to another.
// Used when an agent is transferred between accounts.
func (s *Store) MoveAgentAvatar(ctx context.Context, oldAccount, newAccount, name string) error {
	src := agentAvatarKey(oldAccount, name)
	dst := agentAvatarKey(newAccount, name)
	if err := s.backend.Copy(ctx, src, dst); err != nil {
		return fmt.Errorf("copy agent avatar %s -> %s: %w", src, dst, err)
	}
	if err := s.backend.Delete(ctx, src); err != nil {
		return fmt.Errorf("delete old agent avatar %s: %w", src, err)
	}
	return nil
}

// MoveAgentAvatars moves all agent avatars that exist for a given account to a
// new account name. Agents without avatars are silently skipped.
func (s *Store) MoveAgentAvatars(ctx context.Context, oldAccount, newAccount string, agentNames []string) error {
	for _, name := range agentNames {
		exists, err := s.backend.Exists(ctx, agentAvatarKey(oldAccount, name))
		if err != nil {
			return fmt.Errorf("check agent avatar %s/%s: %w", oldAccount, name, err)
		}
		if !exists {
			continue
		}
		if err := s.MoveAgentAvatar(ctx, oldAccount, newAccount, name); err != nil {
			return err
		}
	}
	return nil
}

// MoveAllForAccount moves the account avatar and all specified agent avatars
// from oldAccount to newAccount. Avatars that don't exist are silently skipped.
// This is the single entry point for account renames and org rename events.
func (s *Store) MoveAllForAccount(ctx context.Context, oldAccount, newAccount string, agentNames []string) error {
	if exists, _ := s.AvatarExists(ctx, oldAccount); exists {
		if err := s.Move(ctx, oldAccount, newAccount); err != nil {
			return fmt.Errorf("move account avatar: %w", err)
		}
	}
	if len(agentNames) > 0 {
		if err := s.MoveAgentAvatars(ctx, oldAccount, newAccount, agentNames); err != nil {
			return fmt.Errorf("move agent avatars: %w", err)
		}
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
