package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/stenstromen/tinykmetrics/internal/models"
)

func (h *Handlers) HandleNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespaces, err := h.kubeService.ListNamespaces(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.NamespaceList{Namespaces: namespaces})
}

func (h *Handlers) HandlePods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	pods, err := h.kubeService.ListPods(r.Context(), namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.PodList{Pods: pods})
}
