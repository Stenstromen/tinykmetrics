package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/stenstromen/tinykmetrics/internal/models"
	"github.com/stenstromen/tinykmetrics/internal/services"
)

type Handlers struct {
	kubeService   *services.KubernetesService
	influxService *services.InfluxDBService
}

func NewHandlers(k *services.KubernetesService, i *services.InfluxDBService) *Handlers {
	return &Handlers{
		kubeService:   k,
		influxService: i,
	}
}

func (h *Handlers) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	status := models.HealthStatus{
		InfluxDB: h.influxService.CheckHealth(),
	}

	w.Header().Set("Content-Type", "application/json")

	if status.InfluxDB {
		status.Status = "healthy"
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

func (h *Handlers) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}
