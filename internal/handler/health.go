package handler

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ebachmann/go-gin-agent/internal/store"
)

var startTime = time.Now()

// HealthHandler handles the health check endpoint.
type HealthHandler struct {
	db *store.DB
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *store.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Check handles GET /health.
// Verifies database connectivity and reports system stats.
func (h *HealthHandler) Check(c *gin.Context) {
	// Verify SQLite is reachable
	dbOK := true
	if err := h.db.Ping(); err != nil {
		dbOK = false
	}

	// Runtime stats
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	status := http.StatusOK
	statusText := "healthy"
	if !dbOK {
		status = http.StatusServiceUnavailable
		statusText = "unhealthy"
	}

	c.JSON(status, gin.H{
		"status":        statusText,
		"uptime":        time.Since(startTime).String(),
		"version":       "0.1.0",
		"database":      boolToStatus(dbOK),
		"goroutines":    runtime.NumGoroutine(),
		"memory_alloc":  formatBytes(mem.Alloc),
		"memory_sys":    formatBytes(mem.Sys),
		"gc_runs":       mem.NumGC,
	})
}

func boolToStatus(ok bool) string {
	if ok {
		return "connected"
	}
	return "disconnected"
}

func formatBytes(b uint64) string {
	const mb = 1024 * 1024
	return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
}
