package services

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/stenstromen/tinykmetrics/internal/models"
)

type InfluxDBService struct {
	Client influxdb2.Client
	Org    string
	Bucket string
}

func NewInfluxDBService(url, token, org, bucket string) *InfluxDBService {
	return &InfluxDBService{
		Client: influxdb2.NewClient(url, token),
		Org:    org,
		Bucket: bucket,
	}
}

func (s *InfluxDBService) CheckHealth() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, err := s.Client.Health(ctx)
	if err != nil {
		return false
	}
	return ok.Status == "pass"
}

func (s *InfluxDBService) QueryMetrics(ctx context.Context, query models.MetricsQuery) (interface{}, error) {
	queryAPI := s.Client.QueryAPI(s.Org)

	fluxQuery := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: -%s)
		|> filter(fn: (r) => r._measurement == "pod_metrics")`,
		s.Bucket, query.Start)

	if query.Namespace != "" {
		fluxQuery += fmt.Sprintf(` |> filter(fn: (r) => r.namespace == "%s")`, query.Namespace)
	}
	if query.Pod != "" {
		fluxQuery += fmt.Sprintf(` |> filter(fn: (r) => r.pod == "%s")`, query.Pod)
	}

	result, err := queryAPI.Query(ctx, fluxQuery)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	var metrics []map[string]interface{}
	for result.Next() {
		metrics = append(metrics, map[string]interface{}{
			"time":      result.Record().Time(),
			"value":     result.Record().Value(),
			"field":     result.Record().Field(),
			"namespace": result.Record().ValueByKey("namespace"),
			"pod":       result.Record().ValueByKey("pod"),
			"container": result.Record().ValueByKey("container"),
		})
	}

	return metrics, result.Err()
}
