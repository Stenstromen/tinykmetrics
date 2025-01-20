package models

type HealthStatus struct {
	InfluxDB bool   `json:"influxdb"`
	Status   string `json:"status"`
}
