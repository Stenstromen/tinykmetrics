package models

type MetricsQuery struct {
	Start     string `json:"start"`
	Stop      string `json:"stop"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}
