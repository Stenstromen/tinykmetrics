package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/stenstromen/tinykmetrics/internal/models"
)

func (h *Handlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	var query models.MetricsQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	metrics, err := h.influxService.QueryMetrics(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
