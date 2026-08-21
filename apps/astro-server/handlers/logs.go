package handlers

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

var k8sTimestampRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$`)
var errorKeywordRE = regexp.MustCompile(`(?i)\b(error|fatal|panic|exception)\b`)

// logPrefixRE matches a leading ISO timestamp with an optional level word.
// Handles both T-separated (2026-04-22T00:45:01.368Z) and space-separated
// (2026-04-22 00:45:01,368) variants.
var logPrefixRE = regexp.MustCompile(
	`^` +
		`\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?Z?` + // timestamp
		`\s+` +
		`(?:(?i:trace|debug|info|warn(?:ing)?|error|err|fatal|crit(?:ical)?)\s+)?`, // optional level word
)

// stripLogPrefix removes a leading timestamp + level prefix from a log message
// when structured timestamp and level are already available separately.
func stripLogPrefix(msg string) string {
	if loc := logPrefixRE.FindStringIndex(msg); loc != nil {
		return msg[loc[1]:]
	}
	return msg
}

func getTimezoneLocation(c *gin.Context) *time.Location {
	if tz := c.Query("timezone"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.UTC
}

type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func lokiLineToEntry(ll loki.LogLine, loc *time.Location) logEntry {
	msg := stripLogPrefix(strings.TrimRight(ll.Line, "\n"))
	return logEntry{
		Timestamp: ll.Timestamp.In(loc).Format(time.RFC3339Nano),
		Level:     ll.Level,
		Message:   msg,
	}
}

// streamLogs writes pod logs to the gin response. It queries Loki if a client
// is configured, otherwise falls back to direct K8s pod log streaming.
// Returns without writing if neither backend is available (503).
func streamLogs(
	c *gin.Context,
	log *logger.Logger,
	lokiClient *loki.Client,
	lokiParams loki.QueryParams,
	k8sClient k8s.ClusterClient,
	namespace, podName string,
	logOpts *corev1.PodLogOptions,
	loc *time.Location,
) {
	if lokiClient != nil {
		lines, err := lokiClient.QueryLogs(c.Request.Context(), lokiParams)
		if err != nil {
			log.Error("logs: query Loki logs failed", "error", err, "namespace", namespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query logs", "details": err.Error()})
			return
		}
		entries := make([]logEntry, 0, len(lines))
		for _, l := range lines {
			entries = append(entries, lokiLineToEntry(l, loc))
		}
		c.JSON(http.StatusOK, entries)
		return
	}

	if k8sClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "log backend not configured"})
		return
	}

	stream, err := k8sClient.Clientset().CoreV1().Pods(namespace).GetLogs(podName, logOpts).Stream(c.Request.Context())
	if err != nil {
		log.Error("logs: get pod logs failed", "error", err, "pod", podName)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get pod logs", "details": err.Error()})
		return
	}
	defer stream.Close() //nolint:errcheck

	logBytes, err := io.ReadAll(stream)
	if err != nil {
		log.Error("logs: read pod logs failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read pod logs"})
		return
	}

	rawLines := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
	entries := make([]logEntry, 0, len(rawLines))
	for _, line := range rawLines {
		if line == "" {
			continue
		}
		entry := logEntry{Message: line}
		if m := k8sTimestampRE.FindStringSubmatch(line); m != nil {
			if t, err := time.Parse(time.RFC3339Nano, m[1]); err == nil {
				entry.Timestamp = t.In(loc).Format(time.RFC3339Nano)
			} else {
				entry.Timestamp = m[1]
			}
			entry.Message = stripLogPrefix(m[2])
		} else {
			entry.Message = stripLogPrefix(line)
		}
		entries = append(entries, entry)
	}

	// Best-effort level filtering for K8s logs (no structured level available).
	if lokiParams.LevelFilter != "" {
		filtered := make([]logEntry, 0, len(entries))
		for _, e := range entries {
			if errorKeywordRE.MatchString(e.Message) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Reverse for backward direction (K8s returns oldest-first).
	if lokiParams.Direction == "backward" {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}

	c.JSON(http.StatusOK, entries)
}
