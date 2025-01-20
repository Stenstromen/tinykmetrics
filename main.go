package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Config struct {
	InfluxURL      string
	InfluxToken    string
	InfluxOrg      string
	InfluxBucket   string
	KubeconfigPath string
	PollInterval   time.Duration
	ListenAddr     string
}

// Add new struct for health status
type HealthStatus struct {
	InfluxDB bool   `json:"influxdb"`
	Status   string `json:"status"`
}

func main() {
	cfg := parseFlags()

	// Initialize Kubernetes clients
	config, err := getKubeConfig(cfg.KubeconfigPath)
	if err != nil {
		log.Fatalf("Error getting Kubernetes config: %v", err)
	}

	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating metrics client: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// Initialize InfluxDB client
	influxClient := influxdb2.NewClient(cfg.InfluxURL, cfg.InfluxToken)
	defer influxClient.Close()
	writeAPI := influxClient.WriteAPIBlocking(cfg.InfluxOrg, cfg.InfluxBucket)

	// Start HTTP server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", handleIndex)
		mux.HandleFunc("/api/metrics", handleMetrics(influxClient, cfg.InfluxOrg, cfg.InfluxBucket))
		mux.HandleFunc("/api/namespaces", handleNamespaces(kubeClient))
		mux.HandleFunc("/api/pods", handlePods(kubeClient))
		mux.HandleFunc("/ready", handleReadiness(influxClient)) // Add readiness probe
		mux.HandleFunc("/status", handleLiveness)               // Add liveness probe

		log.Printf("Starting web server on %s", cfg.ListenAddr)
		if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
			log.Fatalf("Error starting web server: %v", err)
		}
	}()

	// Start metrics collection loop
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	log.Printf("Starting metrics collection every %v", cfg.PollInterval)
	for range ticker.C {
		if err := collectMetrics(metricsClient, writeAPI); err != nil {
			log.Printf("Error collecting metrics: %v", err)
		}
	}
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.InfluxURL, "influx-url", "http://localhost:8086", "InfluxDB URL")
	flag.StringVar(&cfg.InfluxToken, "influx-token", "", "InfluxDB authentication token")
	flag.StringVar(&cfg.InfluxOrg, "influx-org", "default", "InfluxDB organization")
	flag.StringVar(&cfg.InfluxBucket, "influx-bucket", "k8s", "InfluxDB bucket")
	flag.StringVar(&cfg.KubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flag.DurationVar(&cfg.PollInterval, "interval", 30*time.Second, "Metrics collection interval")
	flag.StringVar(&cfg.ListenAddr, "listen-addr", ":8080", "Web server listen address")
	flag.Parse()

	// Validate required flags
	if cfg.InfluxToken == "" {
		log.Fatal("InfluxDB token is required. Please provide it using --influx-token flag")
	}

	return cfg
}

func getKubeConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	return rest.InClusterConfig()
}

func collectMetrics(client *metricsv.Clientset, writeAPI api.WriteAPIBlocking) error {
	ctx := context.Background()

	// Get node metrics
	nodeMetrics, err := client.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting node metrics: %v", err)
	}

	// Get pod metrics
	podMetrics, err := client.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting pod metrics: %v", err)
	}

	now := time.Now()

	// Write node metrics
	for _, node := range nodeMetrics.Items {
		p := influxdb2.NewPoint(
			"node_metrics",
			map[string]string{
				"node": node.Name,
			},
			map[string]interface{}{
				"cpu_usage":    node.Usage.Cpu().MilliValue(),
				"memory_usage": node.Usage.Memory().Value(),
			},
			now,
		)
		if err := writeAPI.WritePoint(ctx, p); err != nil {
			log.Printf("Error writing node metrics: %v", err)
		}
	}

	// Write pod metrics
	for _, pod := range podMetrics.Items {
		for _, container := range pod.Containers {
			p := influxdb2.NewPoint(
				"pod_metrics",
				map[string]string{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"container": container.Name,
				},
				map[string]interface{}{
					"cpu_usage":    container.Usage.Cpu().MilliValue(),
					"memory_usage": container.Usage.Memory().Value(),
				},
				now,
			)
			if err := writeAPI.WritePoint(ctx, p); err != nil {
				log.Printf("Error writing pod metrics: %v", err)
			}
		}
	}

	return nil
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

type MetricsQuery struct {
	Start     string `json:"start"`
	Stop      string `json:"stop"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

func handleMetrics(client influxdb2.Client, org, bucket string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var query MetricsQuery
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		queryAPI := client.QueryAPI(org)
		flux := fmt.Sprintf(`
			from(bucket: "%s")
			|> range(start: -%s)
			|> filter(fn: (r) => r["_measurement"] == "pod_metrics")
		`, bucket, query.Start)

		if query.Namespace != "" {
			flux += fmt.Sprintf(`|> filter(fn: (r) => r["namespace"] == "%s")`, query.Namespace)
		}
		if query.Pod != "" {
			flux += fmt.Sprintf(`|> filter(fn: (r) => r["pod"] == "%s")`, query.Pod)
		}

		result, err := queryAPI.Query(r.Context(), flux)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer result.Close()

		// Convert results to JSON
		var metrics []map[string]interface{}
		for result.Next() {
			record := make(map[string]interface{})
			record["time"] = result.Record().Time()
			record["value"] = result.Record().Value()
			record["field"] = result.Record().Field()
			for k, v := range result.Record().Values() {
				record[k] = v
			}
			metrics = append(metrics, record)
		}

		json.NewEncoder(w).Encode(metrics)
	}
}

type NamespaceList struct {
	Namespaces []string `json:"namespaces"`
}

// @Summary List all namespaces
// @Description Returns a list of all Kubernetes namespaces
// @Produce json
// @Success 200 {object} NamespaceList
// @Failure 500 {string} string "Internal server error"
// @Router /api/namespaces [get]
func handleNamespaces(client *kubernetes.Clientset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		namespaces, err := client.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var namespaceList []string
		for _, ns := range namespaces.Items {
			namespaceList = append(namespaceList, ns.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(NamespaceList{Namespaces: namespaceList})
	}
}

type Pod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type PodList struct {
	Pods []Pod `json:"pods"`
}

// @Summary List all pods
// @Description Returns a list of all pods across all namespaces or in a specific namespace
// @Produce json
// @Param namespace query string false "Optional namespace filter"
// @Success 200 {object} PodList
// @Failure 500 {string} string "Internal server error"
// @Router /api/pods [get]
func handlePods(client *kubernetes.Clientset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		namespace := r.URL.Query().Get("namespace")
		var pods []Pod

		if namespace != "" {
			// List pods in specific namespace
			podList, err := client.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, pod := range podList.Items {
				pods = append(pods, Pod{Name: pod.Name, Namespace: pod.Namespace})
			}
		} else {
			// List pods in all namespaces
			podList, err := client.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, pod := range podList.Items {
				pods = append(pods, Pod{Name: pod.Name, Namespace: pod.Namespace})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PodList{Pods: pods})
	}
}

// Add these new handler functions
func handleReadiness(client influxdb2.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := HealthStatus{
			InfluxDB: checkInfluxDBHealth(client),
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
}

func handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}

func checkInfluxDBHealth(client influxdb2.Client) bool {
	// Try to ping InfluxDB with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use health API to check InfluxDB status
	ok, err := client.Health(ctx)
	if err != nil {
		log.Printf("InfluxDB health check failed: %v", err)
		return false
	}
	return ok.Status == "pass"
}
