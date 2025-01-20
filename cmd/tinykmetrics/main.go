package main

import (
	"log"
	"net/http"

	"github.com/stenstromen/tinykmetrics/internal/config"
	"github.com/stenstromen/tinykmetrics/internal/handlers"
	"github.com/stenstromen/tinykmetrics/internal/services"
	"github.com/stenstromen/tinykmetrics/pkg/utils"
)

func main() {
	cfg := config.ParseFlags()

	// Initialize services
	kubeConfig, err := utils.GetKubeConfig(cfg.KubeconfigPath)
	if err != nil {
		log.Fatalf("Error getting Kubernetes config: %v", err)
	}

	kubeService, err := services.NewKubernetesService(kubeConfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes service: %v", err)
	}

	influxService := services.NewInfluxDBService(
		cfg.InfluxURL,
		cfg.InfluxToken,
		cfg.InfluxOrg,
		cfg.InfluxBucket,
	)
	defer influxService.Client.Close()

	// Initialize handlers
	h := handlers.NewHandlers(kubeService, influxService)

	// Setup routes
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	mux.HandleFunc("/api/metrics", h.HandleMetrics)
	mux.HandleFunc("/api/namespaces", h.HandleNamespaces)
	mux.HandleFunc("/api/pods", h.HandlePods)
	mux.HandleFunc("/ready", h.HandleReadiness)
	mux.HandleFunc("/status", h.HandleLiveness)

	// Start metrics collection
	go kubeService.StartMetricsCollection(cfg.PollInterval, influxService)

	// Start server
	log.Printf("Starting web server on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("Error starting web server: %v", err)
	}
}
