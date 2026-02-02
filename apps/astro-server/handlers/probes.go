package handlers

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

// ProbeHandler manages Kubernetes-style health probes
type ProbeHandler struct {
	log        *logger.Logger
	agentIndex *agentindex.Index
	k8sClient  *k8s.EKSClient
	ready      atomic.Bool
	startTime  time.Time
}

// NewProbeHandler creates a new probe handler
func NewProbeHandler(log *logger.Logger, agentIndex *agentindex.Index, k8sClient *k8s.EKSClient) *ProbeHandler {
	h := &ProbeHandler{
		log:        log,
		agentIndex: agentIndex,
		k8sClient:  k8sClient,
		startTime:  time.Now(),
	}
	h.ready.Store(true)
	return h
}

// SetReady sets the readiness state
func (h *ProbeHandler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Livez returns a handler for liveness probes
// Liveness indicates if the application is running and not deadlocked
// If this fails, Kubernetes will restart the container
func (h *ProbeHandler) Livez() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	}
}

// Readyz returns a handler for readiness probes
// Readiness indicates if the application is ready to serve traffic
// If this fails, Kubernetes will stop sending traffic to this pod
func (h *ProbeHandler) Readyz() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if manually marked as not ready (e.g., during shutdown)
		if !h.ready.Load() {
			c.String(http.StatusServiceUnavailable, "not ready: shutting down")
			return
		}

		// Check agent index database connectivity
		if h.agentIndex != nil {
			if _, err := h.agentIndex.List(); err != nil {
				h.log.Error("Readiness check failed: agent index unavailable", "error", err)
				c.String(http.StatusServiceUnavailable, "not ready: database unavailable")
				return
			}
		}

		c.String(http.StatusOK, "ok")
	}
}

// Healthz returns a handler for combined health checks (verbose)
// Returns detailed health information for debugging
func (h *ProbeHandler) Healthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := make(map[string]string)
		allHealthy := true

		// Check 1: Process is alive (always passes if we get here)
		checks["process"] = "ok"

		// Check 2: Agent index database
		if h.agentIndex != nil {
			if _, err := h.agentIndex.List(); err != nil {
				checks["database"] = "failed: " + err.Error()
				allHealthy = false
			} else {
				checks["database"] = "ok"
			}
		} else {
			checks["database"] = "not configured"
		}

		// Check 3: Kubernetes API connectivity (informational, does not affect overall health)
		if h.k8sClient != nil {
			if version, err := h.k8sClient.GetServerVersion(); err != nil {
				checks["kubernetes"] = "failed: " + err.Error()
			} else {
				checks["kubernetes"] = "ok (version: " + version + ")"
			}
		} else {
			checks["kubernetes"] = "not configured"
		}

		// Check 4: Ready state
		if h.ready.Load() {
			checks["ready"] = "ok"
		} else {
			checks["ready"] = "not ready"
			allHealthy = false
		}

		status := http.StatusOK
		statusText := "healthy"
		if !allHealthy {
			status = http.StatusServiceUnavailable
			statusText = "unhealthy"
		}

		c.JSON(status, gin.H{
			"status":    statusText,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptime":    time.Since(h.startTime).String(),
			"checks":    checks,
		})
	}
}
