package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	controlapi "github.com/moby/buildkit/api/services/control"
	"google.golang.org/protobuf/proto"
)

// vertexState tracks the lifecycle of a single BuildKit vertex so we can
// emit elapsed-time annotations on completion and "still running" heartbeats
// while a vertex is in flight.
type vertexState struct {
	name      string
	started   time.Time
	completed bool
}

// heartbeatInterval controls how long streamBuildOutput will sit silent
// before printing a "… still running" line. Verbose mode tightens the
// interval so users debugging a stuck step see progress sooner.
func heartbeatInterval(verbose bool) time.Duration {
	if verbose {
		return 5 * time.Second
	}
	return 15 * time.Second
}

// formatElapsed renders a duration as "12.3s" under a minute and "1m 04s"
// otherwise. Used for both per-vertex completion lines and heartbeats.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm %02ds", minutes, seconds)
}

// truncateVertexName clips a build-step name to keep heartbeat lines
// readable. BuildKit names like "[frontend-builder 6/6] RUN bun run build"
// fit fine; long secrets-mounted RUN expressions don't.
func truncateVertexName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	return name[:max-1] + "…"
}

// streamBuildOutput consumes the streaming JSON response from a Docker
// ImageBuild call and translates it into terminal output. Three modes:
//   - quiet: suppress all output
//   - default: print each vertex name on first sighting; emit a completion
//     line with elapsed time for any vertex that took 5s+; emit a
//     heartbeat after 15s of silence so users can tell a long step (e.g.
//     vite transforming 5k modules in Docker on Apple Silicon, ~7 min)
//     isn't wedged
//   - verbose: tighter heartbeat (5s), emit completion timing for every
//     vertex regardless of duration, and stamp the BuildKit log lines so
//     it's clear which vertex emitted them
func streamBuildOutput(reader io.Reader, verbose, quiet bool) error {
	if !quiet {
		fmt.Println()
	}

	var (
		mu        sync.Mutex
		vertices  = make(map[string]*vertexState)
		activity  = time.Now()
		lastError string
	)

	if !quiet {
		stopHeartbeat := startHeartbeat(&mu, vertices, &activity, verbose)
		defer stopHeartbeat()
	}

	decoder := json.NewDecoder(reader)
	for {
		var msg struct {
			ID     string `json:"id"`
			Aux    string `json:"aux"`
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		// Any inbound message counts as activity, suppressing the heartbeat.
		mu.Lock()
		activity = time.Now()
		mu.Unlock()

		if msg.Error != "" {
			lastError = msg.Error
			if !quiet {
				fmt.Printf("      %sERROR: %s%s\n", colorRed, msg.Error, colorReset)
			}
		}

		if msg.Stream != "" && !quiet {
			fmt.Print(msg.Stream)
		}

		if msg.ID == "moby.buildkit.trace" && msg.Aux != "" && !quiet {
			data, err := base64.StdEncoding.DecodeString(msg.Aux)
			if err != nil {
				continue
			}
			var status controlapi.StatusResponse
			if err := proto.Unmarshal(data, &status); err != nil {
				continue
			}
			lastError = handleTrace(&mu, vertices, &status, verbose, lastError)
		}
	}

	if lastError != "" {
		return fmt.Errorf("build failed: %s", lastError)
	}
	return nil
}

// handleTrace processes one BuildKit StatusResponse: emits new vertex
// names, completion timings, and dimmed log lines. Returns the most recent
// error seen so the caller can keep tracking it across multiple traces.
func handleTrace(mu *sync.Mutex, vertices map[string]*vertexState, status *controlapi.StatusResponse, verbose bool, lastError string) string {
	mu.Lock()
	defer mu.Unlock()

	for _, v := range status.Vertexes {
		digest := v.Digest
		vs, exists := vertices[digest]
		if !exists && v.Name != "" {
			vs = &vertexState{name: v.Name}
			vertices[digest] = vs
			fmt.Printf("      %s%s%s\n", colorCyan, v.Name, colorReset)
		}
		if vs == nil {
			continue
		}
		if vs.started.IsZero() && v.Started != nil {
			vs.started = v.Started.AsTime()
		}
		if !vs.completed && v.Completed != nil {
			vs.completed = true
			elapsed := time.Duration(0)
			if !vs.started.IsZero() {
				elapsed = v.Completed.AsTime().Sub(vs.started)
			}
			// Only annotate slow steps by default so fast COPY/FROM lines
			// don't drown out the build script output the user actually
			// cares about. Verbose flips the gate so every step is timed.
			if verbose || elapsed >= 5*time.Second {
				fmt.Printf("      %s  ✓ %s (%s)%s\n", colorDim, vs.name, formatElapsed(elapsed), colorReset)
			}
		}
		if v.Error != "" {
			lastError = v.Error
			fmt.Printf("      %sERROR: %s%s\n", colorRed, v.Error, colorReset)
		}
	}

	for _, l := range status.Logs {
		if len(l.Msg) == 0 {
			continue
		}
		prefix := ""
		// In verbose mode, tag each log line with the (truncated) vertex
		// name so users can disambiguate parallel build stages whose
		// output would otherwise interleave silently.
		if verbose {
			if vs, ok := vertices[l.Vertex]; ok {
				prefix = "[" + truncateVertexName(vs.name, 28) + "] "
			}
		}
		for _, line := range strings.Split(string(l.Msg), "\n") {
			if line != "" {
				fmt.Printf("      %s%s%s%s\n", colorDim, prefix, line, colorReset)
			}
		}
	}

	return lastError
}

// startHeartbeat spawns a goroutine that, on every tick, prints a
// "still running" line listing the currently in-flight vertex names with
// their per-vertex elapsed time, but only if no other output has happened
// for at least one full interval. Returns a stop function the caller must
// defer to terminate the goroutine cleanly.
func startHeartbeat(mu *sync.Mutex, vertices map[string]*vertexState, activity *time.Time, verbose bool) func() {
	interval := heartbeatInterval(verbose)
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				mu.Lock()
				silent := now.Sub(*activity)
				if silent < interval {
					mu.Unlock()
					continue
				}
				var inflight []string
				for _, v := range vertices {
					if v.completed || v.started.IsZero() {
						continue
					}
					inflight = append(inflight, fmt.Sprintf("%s (%s)", truncateVertexName(v.name, 40), formatElapsed(now.Sub(v.started))))
				}
				mu.Unlock()
				if len(inflight) > 0 {
					fmt.Printf("      %s… still running %s%s\n", colorDim, strings.Join(inflight, ", "), colorReset)
				}
			}
		}
	}()

	return func() {
		ticker.Stop()
		close(done)
	}
}
