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

	"github.com/docker/docker/api/types/registry"
	"github.com/moby/moby/client"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

const maxPushRetries = 3

// getDockerRegistryAuth returns a base64-encoded registry auth string
// for use with the Docker Engine API.
// If tokenOverride is non-empty, it is used directly instead of fetching from the profile.
func getDockerRegistryAuth(tokenOverride string) (string, error) {
	var token string
	if tokenOverride != "" {
		token = tokenOverride
	} else {
		tokenManager := auth.NewTokenManager(binaryName)
		var err error
		token, err = tokenManager.GetValidAccessToken(context.Background())
		if err != nil {
			return "", fmt.Errorf("failed to get access token: %w", err)
		}
	}

	authConfig := registry.AuthConfig{
		RegistryToken: token,
	}
	authBytes, err := json.Marshal(authConfig) //nolint:gosec // registry auth token, not a secret leak
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
		pushResp, err := dockerCli.ImagePush(ctx, imageRef, client.ImagePushOptions{
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
// If tokenOverride is non-empty, it is used for auth instead of the stored profile token.
func pushImageToRegistryStreaming(localImageName, remoteImageName string, skipAuth bool, tokenOverride string) (int64, error) {
	fmt.Printf("  %sstreaming...%s", colorDim, colorReset)

	ctx := context.Background()

	dockerCli, err := newDockerClient()
	if err != nil {
		fmt.Println()
		return 0, err
	}

	// Tag image for remote registry
	if _, err := dockerCli.ImageTag(ctx, client.ImageTagOptions{Source: localImageName, Target: remoteImageName}); err != nil {
		fmt.Println()
		return 0, fmt.Errorf("failed to tag image %s -> %s: %w", localImageName, remoteImageName, err)
	}

	// Get auth
	var authStr string
	if !skipAuth {
		authStr, err = getDockerRegistryAuth(tokenOverride)
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
