package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/moby/moby/client"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
)

const maxPushRetries = 3

// getDockerRegistryAuth returns a base64-encoded registry auth string
// for use with the Docker Engine API.
func getDockerRegistryAuth() (string, error) {
	tokenManager := auth.NewTokenManager(binaryName)
	token, err := tokenManager.GetValidAccessToken(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	authConfig := registry.AuthConfig{
		RegistryToken: token,
	}
	authBytes, err := json.Marshal(authConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth config: %w", err)
	}

	return base64.URLEncoding.EncodeToString(authBytes), nil
}

// dockerPushWithRetry pushes an image via Docker API with retry on transient errors (502, 503, etc).
// On retry, Docker skips already-pushed layers and only re-attempts the failed ones.
func dockerPushWithRetry(ctx context.Context, dockerCli *client.Client, imageRef, authStr string, label string) (int64, error) {
	var lastErr error
	for attempt := 1; attempt <= maxPushRetries; attempt++ {
		pushResp, err := dockerCli.ImagePush(ctx, imageRef, dockerimage.PushOptions{
			RegistryAuth: authStr,
		})
		if err != nil {
			return 0, fmt.Errorf("failed to initiate push: %w", err)
		}

		totalBytes, err := streamDockerPushProgress(pushResp, label)
		_ = pushResp.Close()

		if err == nil {
			return totalBytes, nil
		}

		lastErr = err
		isRetryable := strings.Contains(err.Error(), "502") ||
			strings.Contains(err.Error(), "503") ||
			strings.Contains(err.Error(), "504") ||
			strings.Contains(err.Error(), "Bad Gateway") ||
			strings.Contains(err.Error(), "Service Unavailable") ||
			strings.Contains(err.Error(), "Gateway Timeout")

		if !isRetryable || attempt == maxPushRetries {
			break
		}

		wait := time.Duration(attempt*2) * time.Second
		fmt.Fprintf(os.Stderr, "\n  %sretrying in %s (attempt %d/%d)...%s\n", colorYellow, wait, attempt, maxPushRetries, colorReset)
		time.Sleep(wait)
	}

	return 0, lastErr
}

// pushImageToRegistryStreaming pushes an image using Docker Engine API streaming.
// Image data flows directly from Docker daemon to registry without loading into Go memory.
func pushImageToRegistryStreaming(localImageName, remoteImageName string, skipAuth bool) (int64, error) {
	fmt.Printf("  %sstreaming...%s", colorDim, colorReset)

	ctx := context.Background()

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerCli.Close() //nolint:errcheck

	// Tag image for remote registry
	if err := dockerCli.ImageTag(ctx, localImageName, remoteImageName); err != nil {
		fmt.Println()
		return 0, fmt.Errorf("failed to tag image %s -> %s: %w", localImageName, remoteImageName, err)
	}

	// Get auth
	var authStr string
	if !skipAuth {
		authStr, err = getDockerRegistryAuth()
		if err != nil {
			fmt.Println()
			return 0, fmt.Errorf("failed to get registry auth: %w", err)
		}
	}

	fmt.Print("\r                    \r")

	// Push with retry support for transient registry errors
	totalBytes, err := dockerPushWithRetry(ctx, dockerCli, remoteImageName, authStr, "pushing")
	if err != nil {
		return 0, fmt.Errorf("failed to push image: %w", err)
	}

	return totalBytes, nil
}

// pushMultiPlatformToRegistryStreaming pushes platform images using Docker Engine API
// streaming, then creates a manifest list. Image data never enters Go process memory.
// Platforms are pushed sequentially to avoid garbled progress output and Docker daemon contention.
func pushMultiPlatformToRegistryStreaming(baseName, tag, remoteImageName string, platforms []string, skipAuth bool) (int64, error) {
	fmt.Println()

	ctx := context.Background()

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return 0, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerCli.Close() //nolint:errcheck

	// Get auth once for all pushes
	var authStr string
	if !skipAuth {
		authStr, err = getDockerRegistryAuth()
		if err != nil {
			return 0, fmt.Errorf("failed to get registry auth: %w", err)
		}
	}

	type platformResult struct {
		platform  string
		remoteRef string
	}

	// Push each platform sequentially for clean progress output
	var results []platformResult
	var totalBytes int64

	for _, platform := range platforms {
		parts := strings.SplitN(platform, "/", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid platform format %q, expected os/arch", platform)
		}

		localTag := platformImageTag(baseName, tag, platform)
		platformRemoteTag := fmt.Sprintf("%s-%s", remoteImageName, strings.ReplaceAll(platform, "/", "-"))

		// Tag for remote
		if err := dockerCli.ImageTag(ctx, localTag, platformRemoteTag); err != nil {
			return 0, fmt.Errorf("failed to tag %s -> %s: %w", localTag, platformRemoteTag, err)
		}

		// Stream push via Docker API with retry - no Go memory needed for image data
		bytes, err := dockerPushWithRetry(ctx, dockerCli, platformRemoteTag, authStr, platform)
		if err != nil {
			return 0, fmt.Errorf("push failed for %s: %w", platform, err)
		}
		totalBytes += bytes

		results = append(results, platformResult{
			platform:  platform,
			remoteRef: platformRemoteTag,
		})
	}

	fmt.Printf("  %screating manifest list...%s", colorDim, colorReset)

	// Create manifest list by fetching lightweight descriptors from registry
	// (images are already pushed, so we only need the manifests - no full image data)
	var addendums []mutate.IndexAddendum
	var totalSize int64

	for _, pr := range results {
		parts := strings.SplitN(pr.platform, "/", 2)
		if len(parts) != 2 {
			continue
		}

		ref, err := name.ParseReference(pr.remoteRef)
		if err != nil {
			return 0, fmt.Errorf("failed to parse ref %s: %w", pr.remoteRef, err)
		}

		var opts []remote.Option
		if !skipAuth {
			opts = append(opts, remote.WithAuth(auth.GetCraneAuth(binaryName)))
		}
		opts = append(opts, remote.WithTransport(getOptimizedTransport()))

		// Fetch only the descriptor from registry (lightweight, just manifest metadata)
		desc, err := remote.Get(ref, opts...)
		if err != nil {
			return 0, fmt.Errorf("failed to get descriptor for %s: %w", pr.remoteRef, err)
		}
		totalSize += desc.Size

		img, err := desc.Image()
		if err != nil {
			return 0, fmt.Errorf("failed to get image from descriptor: %w", err)
		}

		addendums = append(addendums, mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           parts[0],
					Architecture: parts[1],
				},
			},
		})
	}

	// Create and push the manifest list (tiny payload, just JSON)
	index := mutate.AppendManifests(empty.Index, addendums...)

	finalRef, err := name.ParseReference(remoteImageName)
	if err != nil {
		return 0, fmt.Errorf("failed to parse final reference: %w", err)
	}

	var opts []remote.Option
	if !skipAuth {
		opts = append(opts, remote.WithAuth(auth.GetCraneAuth(binaryName)))
	}
	opts = append(opts, remote.WithTransport(getOptimizedTransport()))

	if err := remote.WriteIndex(finalRef, index, opts...); err != nil {
		return 0, fmt.Errorf("failed to write manifest list: %w", err)
	}

	fmt.Print("\r                              \r")

	return totalSize, nil
}

// streamDockerPushProgress reads Docker push progress events and displays
// per-layer progress lines that update in place, similar to `docker push`.
func streamDockerPushProgress(reader io.ReadCloser, label string) (int64, error) {
	decoder := json.NewDecoder(reader)

	if label == "" {
		label = "pushing"
	}

	// Per-layer tracking
	type layerInfo struct {
		status  string
		current int64
		total   int64
	}
	layers := make(map[string]*layerInfo)
	layerOrder := []string{}
	numPrinted := 0 // number of lines currently on screen

	const barWidth = 20

	// render redraws all layer lines in place
	render := func() {
		// Move cursor up to the first layer line
		if numPrinted > 0 {
			fmt.Fprintf(os.Stderr, "\033[%dA", numPrinted)
		}

		for _, id := range layerOrder {
			l := layers[id]
			shortID := id
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			// Clear line and write layer status
			fmt.Fprintf(os.Stderr, "\033[K")

			switch {
			case l.status == "Pushing" && l.total > 0:
				// Show progress bar
				pct := float64(l.current) / float64(l.total)
				filled := int(pct * float64(barWidth))
				if filled > barWidth {
					filled = barWidth
				}
				bar := strings.Repeat("=", filled)
				if filled < barWidth {
					bar += ">"
					bar += strings.Repeat(" ", barWidth-filled-1)
				}
				fmt.Fprintf(os.Stderr, "  %s: Pushing  [%s] %s/%s\n",
					shortID, bar, formatBytes(l.current), formatBytes(l.total))

			case l.status == "Pushed":
				fmt.Fprintf(os.Stderr, "  %s: %s%s%s\n", shortID, colorGreen, l.status, colorReset)

			case l.status == "Layer already exists":
				fmt.Fprintf(os.Stderr, "  %s: %s%s%s\n", shortID, colorGreen, l.status, colorReset)

			case strings.HasPrefix(l.status, "Retrying"):
				fmt.Fprintf(os.Stderr, "  %s: %s%s%s\n", shortID, colorYellow, l.status, colorReset)

			default:
				fmt.Fprintf(os.Stderr, "  %s: %s%s%s\n", shortID, colorDim, l.status, colorReset)
			}
		}

		numPrinted = len(layerOrder)
	}

	// Print label
	fmt.Fprintf(os.Stderr, "  %s%s%s\n", colorDim, label, colorReset)
	numPrinted = 0

	for {
		var event struct {
			Status         string `json:"status"`
			ID             string `json:"id"`
			Progress       string `json:"progress"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
			Aux   json.RawMessage `json:"aux"`
			Error string          `json:"error"`
		}

		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("error reading push progress: %w", err)
		}

		if event.Error != "" {
			render() // Final render before error
			if event.ID != "" {
				return 0, fmt.Errorf("[layer %s] %s", event.ID, event.Error)
			}
			return 0, fmt.Errorf("%s", event.Error)
		}

		if event.ID == "" {
			continue
		}

		// Register new layer
		if _, exists := layers[event.ID]; !exists {
			layers[event.ID] = &layerInfo{}
			layerOrder = append(layerOrder, event.ID)
		}

		l := layers[event.ID]
		if event.Status != "" {
			l.status = event.Status
		}
		if event.ProgressDetail.Total > 0 {
			l.current = event.ProgressDetail.Current
			l.total = event.ProgressDetail.Total
		}

		// Reset progress on retry
		if strings.HasPrefix(event.Status, "Retrying") {
			l.current = 0
		}

		render()
	}

	// Calculate total bytes pushed
	var totalBytes int64
	for _, l := range layers {
		if l.total > 0 {
			totalBytes += l.total
		}
	}

	return totalBytes, nil
}

// formatBytes formats bytes into a human-readable string
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
