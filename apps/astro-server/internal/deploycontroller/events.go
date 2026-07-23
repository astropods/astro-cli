package deploycontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

const (
	// eventsListLimit bounds the per-sync events List. Namespaces accumulate many
	// events; we only keep the newest, current-object ones after filtering.
	eventsListLimit = 500
	// maxPersistedEvents caps the snapshot the events endpoint renders.
	maxPersistedEvents = 50
)

// buildDeploymentEvents lists the namespace's Kubernetes events and reduces them
// to the read model behind GET /deployments/:id/events: scoped to the deployment's
// current objects (currentNames), humanized, and sorted newest-first. Events for
// objects no longer present (previous deploys/pods) are dropped; when currentNames
// is empty the scope filter is skipped.
func buildDeploymentEvents(ctx context.Context, cs kubernetes.Interface, namespace string, currentNames map[string]bool) ([]deploymentstore.EventItem, error) {
	list, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{Limit: eventsListLimit})
	if err != nil {
		return nil, fmt.Errorf("list events %s: %w", namespace, err)
	}

	type timed struct {
		item deploymentstore.EventItem
		last time.Time
	}
	var timedItems []timed
	for i := range list.Items {
		evt := &list.Items[i]
		if len(currentNames) > 0 && !currentNames[evt.InvolvedObject.Name] {
			continue
		}
		last := eventLastSeen(evt)
		first := evt.FirstTimestamp.Time
		if first.IsZero() {
			first = evt.EventTime.Time
		}
		if first.IsZero() {
			first = last
		}
		title, guidance, severity, _ := HumanizeEvent(evt.Reason, evt.Message)
		timedItems = append(timedItems, timed{
			item: deploymentstore.EventItem{
				Type:           evt.Type,
				Reason:         evt.Reason,
				Message:        evt.Message,
				ObjectKind:     evt.InvolvedObject.Kind,
				ObjectName:     evt.InvolvedObject.Name,
				Count:          evt.Count,
				FirstTimestamp: first.UTC().Format(time.RFC3339),
				LastTimestamp:  last.UTC().Format(time.RFC3339),
				Title:          title,
				Guidance:       guidance,
				Severity:       severity,
			},
			last: last,
		})
	}

	sort.SliceStable(timedItems, func(i, j int) bool {
		return timedItems[i].last.After(timedItems[j].last)
	})
	if len(timedItems) > maxPersistedEvents {
		timedItems = timedItems[:maxPersistedEvents]
	}

	events := make([]deploymentstore.EventItem, 0, len(timedItems))
	for _, t := range timedItems {
		events = append(events, t.item)
	}
	return events, nil
}

// currentObjectNames collects the names of the workloads and pods observed in a
// runtime snapshot, used to scope events to the deployment's live objects and
// drop noise from previous deploys/pods.
func currentObjectNames(snap deploymentstore.RuntimeSnapshot) map[string]bool {
	names := make(map[string]bool)
	for _, w := range snap.Workloads {
		names[w.Name] = true
		for _, p := range w.Pods {
			names[p.Name] = true
		}
	}
	return names
}

// eventLastSeen resolves the most recent occurrence time of an event. The
// deprecated core/v1 LastTimestamp is zero on events created via the events.k8s.io
// API, which populate EventTime and (for repeats) Series.LastObservedTime — using
// only LastTimestamp/CreationTimestamp would sort an actively-firing event as old.
func eventLastSeen(evt *corev1.Event) time.Time {
	if evt.Series != nil && !evt.Series.LastObservedTime.Time.IsZero() {
		return evt.Series.LastObservedTime.Time
	}
	if !evt.LastTimestamp.Time.IsZero() {
		return evt.LastTimestamp.Time
	}
	if !evt.EventTime.Time.IsZero() {
		return evt.EventTime.Time
	}
	return evt.CreationTimestamp.Time
}

// HumanizeEvent maps a Kubernetes pod event to a plain-language title, guidance,
// and severity for the deployment Events tab and the stuck-deploy banner,
// covering the common working, transient, and error/stuck states. severity is
// "info" (normal progress), "transient" (self-recovering), or "stuck" (needs user
// action); the client's stuck banner triggers on "stuck". ok is false for events
// we have no copy for, in which case the UI falls back to the raw reason/message.
// The message disambiguates reasons covering more than one failure mode (BackOff
// is both crash-loop and image-pull; Failed is both image-pull and other errors).
func HumanizeEvent(reason, message string) (title, guidance, severity string, ok bool) {
	msg := strings.ToLower(message)
	imagePull := func() (string, string, string, bool) {
		return "Action required: Image pull failed",
			"The container image can't be pulled. Check the image name and tag, and that the registry is reachable with valid credentials, then redeploy.",
			"stuck", true
	}
	crashLoop := func() (string, string, string, bool) {
		return "Action required: Container crash looping",
			"The container keeps starting and exiting. This is usually a bad start command or a missing secret or environment variable. Check the pod logs for the crash reason, fix it, then redeploy.",
			"stuck", true
	}

	switch reason {
	// Working — normal progress toward a running agent.
	case "Scheduled":
		return "Scheduled", "Your agent has been assigned to a node.", "info", true
	case "Pulling":
		return "Downloading image", "Fetching your agent's container image — this may take a moment.", "info", true
	case "Pulled":
		return "Image ready", "Your agent's container image is downloaded and ready.", "info", true
	case "Created":
		return "Preparing agent", "Your agent's container has been created.", "info", true
	case "Started":
		return "Starting up", "Your agent is booting and will be ready shortly.", "info", true

	// Transient — self-recovering, no user action needed.
	case "Unhealthy":
		return "Health check pending", "Your agent is still initializing — waiting for it to pass health checks.", "transient", true
	case "BackOff":
		// BackOff is the kubelet backing off before a retry — transient, not a
		// terminal error (the stuck states ImagePullBackOff / CrashLoopBackOff are).
		// Never "Action required"; keep pull/crash flavor for context.
		switch {
		case strings.Contains(msg, "pull"), strings.Contains(msg, "image"):
			return "Retrying image pull", "The image couldn't be pulled yet and is being retried. If it keeps failing, check the image name/tag and that the registry is reachable.", "transient", true
		case strings.Contains(msg, "restart"), strings.Contains(msg, "crash"):
			return "Restarting container", "The container exited and is being restarted. If it keeps happening, check the pod logs for the crash reason.", "transient", true
		default:
			return "Retrying", "A transient issue occurred; the system is retrying automatically.", "transient", true
		}

	// Stuck / error states.
	case "ImagePullBackOff", "ErrImagePull", "ErrImageNeverPull", "InvalidImageName":
		return imagePull()
	case "CrashLoopBackOff":
		return crashLoop()
	case "Failed":
		// Failed is generic; only surface the image-pull variant, otherwise
		// leave it raw for the UI to show the reason/message.
		if strings.Contains(msg, "image") || strings.Contains(msg, "pull") {
			return imagePull()
		}
		return "", "", "", false
	case "FailedScheduling":
		return "Action required: Deployment stuck",
			"This agent requests more CPU/memory than any node has available, so it can't be placed. Reduce its resources under Configure → Advanced sizing and redeploy.",
			"stuck", true
	case "FailedMount", "FailedAttachVolume":
		return "Storage issue", "There was a problem attaching storage; the system will retry.", "transient", true
	}
	return "", "", "", false
}
