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

type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
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
) {
	if lokiClient != nil {
		lines, err := lokiClient.QueryLogs(c.Request.Context(), lokiParams)
		if err != nil {
			log.Error("Failed to query Loki logs", "error", err, "namespace", namespace)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query logs", "details": err.Error()})
			return
		}
		entries := make([]logEntry, 0, len(lines))
		for _, l := range lines {
			entries = append(entries, logEntry{
				Timestamp: l.Timestamp.UTC().Format(time.RFC3339Nano),
				Level:     l.Level,
				Message:   strings.TrimRight(l.Line, "\n"),
			})
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
		log.Error("Failed to get pod logs", "error", err, "pod", podName)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get pod logs", "details": err.Error()})
		return
	}
	defer stream.Close() //nolint:errcheck

	logBytes, err := io.ReadAll(stream)
	if err != nil {
		log.Error("Failed to read pod logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read pod logs"})
		return
	}

	k8sTS := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$`)
	rawLines := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
	entries := make([]logEntry, 0, len(rawLines))
	for _, line := range rawLines {
		if line == "" {
			continue
		}
		entry := logEntry{Message: line}
		if m := k8sTS.FindStringSubmatch(line); m != nil {
			entry.Timestamp = m[1]
			entry.Message = m[2]
		}
		entries = append(entries, entry)
	}
	c.JSON(http.StatusOK, entries)
}
