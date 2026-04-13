package handlers

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

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
		var sb strings.Builder
		for _, l := range lines {
			sb.WriteString(l.Timestamp.UTC().Format(time.RFC3339Nano))
			sb.WriteString(" ")
			sb.WriteString(strings.TrimRight(l.Line, "\n"))
			sb.WriteString("\n")
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(sb.String()))
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
	c.Data(http.StatusOK, "text/plain; charset=utf-8", logBytes)
}
