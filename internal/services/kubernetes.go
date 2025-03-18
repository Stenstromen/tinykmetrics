package services

import (
	"context"
	"fmt"
	"log"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/stenstromen/tinykmetrics/internal/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type KubernetesService struct {
	client        *kubernetes.Clientset
	metricsClient *metricsv.Clientset
	testMode      bool
	firstRun      bool // Track if this is the first collection run
}

func NewKubernetesService(config *rest.Config, testMode bool) (*KubernetesService, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating metrics client: %v", err)
	}

	return &KubernetesService{
		client:        client,
		metricsClient: metricsClient,
		testMode:      testMode,
		firstRun:      true,
	}, nil
}

// NewKubernetesServiceWithFakeClient creates a new KubernetesService with fake clients for testing
func NewKubernetesServiceWithFakeClient(testMode bool) (*KubernetesService, error) {
	// Create empty structs for the clients
	// We don't need real clients in test mode since we'll use mock data
	return &KubernetesService{
		client:        nil,
		metricsClient: nil,
		testMode:      testMode,
		firstRun:      true,
	}, nil
}

func (s *KubernetesService) ListNamespaces(ctx context.Context) ([]string, error) {
	// If in test mode with nil client, return mock namespaces
	if s.client == nil {
		return []string{"default", "kube-system", "monitoring", "database"}, nil
	}

	namespaces, err := s.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var namespaceList []string
	for _, ns := range namespaces.Items {
		namespaceList = append(namespaceList, ns.Name)
	}
	return namespaceList, nil
}

func (s *KubernetesService) ListPods(ctx context.Context, namespace string) ([]models.Pod, error) {
	// If in test mode with nil client, return mock pods
	if s.client == nil {
		mockPods := []models.Pod{
			{Name: "web-app-1", Namespace: "default"},
			{Name: "kube-dns-1", Namespace: "kube-system"},
			{Name: "prometheus-1", Namespace: "monitoring"},
			{Name: "postgres-1", Namespace: "database"},
		}

		// Filter by namespace if specified
		if namespace != "" {
			var filteredPods []models.Pod
			for _, pod := range mockPods {
				if pod.Namespace == namespace {
					filteredPods = append(filteredPods, pod)
				}
			}
			return filteredPods, nil
		}

		return mockPods, nil
	}

	var pods []models.Pod
	podList, err := s.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		pods = append(pods, models.Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		})
	}
	return pods, nil
}

func (s *KubernetesService) StartMetricsCollection(interval time.Duration, influxService *InfluxDBService) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Starting metrics collection every %v", interval)

	// If in test mode, immediately collect mock metrics
	if s.testMode && s.firstRun {
		log.Println("Test mode enabled: collecting mock metrics for first run")
		if err := s.collectMockMetrics(influxService); err != nil {
			log.Printf("Error collecting mock metrics: %v", err)
		}
		s.firstRun = false
	}

	for range ticker.C {
		if s.testMode && s.firstRun {
			if err := s.collectMockMetrics(influxService); err != nil {
				log.Printf("Error collecting mock metrics: %v", err)
			}
			s.firstRun = false
		} else {
			if err := s.collectMetrics(influxService); err != nil {
				log.Printf("Error collecting metrics: %v", err)
			}
		}
	}
}

func (s *KubernetesService) collectMetrics(influxService *InfluxDBService) error {
	// If in test mode with nil clients, use mock metrics instead
	if s.client == nil || s.metricsClient == nil {
		return s.collectMockMetrics(influxService)
	}

	ctx := context.Background()
	now := time.Now()

	// Collect node metrics
	nodeMetrics, err := s.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting node metrics: %v", err)
	}

	// Collect pod metrics
	podMetrics, err := s.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting pod metrics: %v", err)
	}

	writeAPI := influxService.Client.WriteAPIBlocking(influxService.Org, influxService.Bucket)

	// Write node metrics
	for _, node := range nodeMetrics.Items {
		p := influxdb2.NewPoint(
			"node_metrics",
			map[string]string{"node": node.Name},
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

// New method to collect mock metrics
func (s *KubernetesService) collectMockMetrics(influxService *InfluxDBService) error {
	ctx := context.Background()
	now := time.Now()
	writeAPI := influxService.Client.WriteAPIBlocking(influxService.Org, influxService.Bucket)

	// Mock node metrics
	mockNodes := []struct {
		name        string
		cpuUsage    int64
		memoryUsage int64
	}{
		{"node-1", 500, 4 * 1024 * 1024 * 1024},
		{"node-2", 750, 6 * 1024 * 1024 * 1024},
		{"node-3", 300, 2 * 1024 * 1024 * 1024},
	}

	for _, node := range mockNodes {
		p := influxdb2.NewPoint(
			"node_metrics",
			map[string]string{"node": node.name},
			map[string]interface{}{
				"cpu_usage":    node.cpuUsage,
				"memory_usage": node.memoryUsage,
			},
			now,
		)
		if err := writeAPI.WritePoint(ctx, p); err != nil {
			log.Printf("Error writing mock node metrics: %v", err)
		}
	}

	// Mock pod metrics
	mockPods := []struct {
		namespace     string
		podName       string
		containerName string
		cpuUsage      int64
		memoryUsage   int64
	}{
		{"default", "web-app-1", "web-container", 200, 512 * 1024 * 1024},
		{"default", "web-app-1", "sidecar", 50, 128 * 1024 * 1024},
		{"kube-system", "kube-dns-1", "dns", 100, 256 * 1024 * 1024},
		{"monitoring", "prometheus-1", "prometheus", 300, 1024 * 1024 * 1024},
		{"database", "postgres-1", "postgres", 400, 2 * 1024 * 1024 * 1024},
	}

	for _, pod := range mockPods {
		p := influxdb2.NewPoint(
			"pod_metrics",
			map[string]string{
				"namespace": pod.namespace,
				"pod":       pod.podName,
				"container": pod.containerName,
			},
			map[string]interface{}{
				"cpu_usage":    pod.cpuUsage,
				"memory_usage": pod.memoryUsage,
			},
			now,
		)
		if err := writeAPI.WritePoint(ctx, p); err != nil {
			log.Printf("Error writing mock pod metrics: %v", err)
		}
	}

	log.Println("Successfully wrote mock metrics to InfluxDB")
	return nil
}
