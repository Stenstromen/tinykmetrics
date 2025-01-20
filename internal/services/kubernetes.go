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
}

func NewKubernetesService(config *rest.Config) (*KubernetesService, error) {
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
	}, nil
}

func (s *KubernetesService) ListNamespaces(ctx context.Context) ([]string, error) {
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
	for range ticker.C {
		if err := s.collectMetrics(influxService); err != nil {
			log.Printf("Error collecting metrics: %v", err)
		}
	}
}

func (s *KubernetesService) collectMetrics(influxService *InfluxDBService) error {
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
