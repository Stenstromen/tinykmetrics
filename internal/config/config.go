package config

import (
	"flag"
	"log"
	"time"
)

type Config struct {
	InfluxURL      string
	InfluxToken    string
	InfluxOrg      string
	InfluxBucket   string
	KubeconfigPath string
	PollInterval   time.Duration
	ListenAddr     string
	TestMode       bool
}

func ParseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.InfluxURL, "influx-url", "http://localhost:8086", "InfluxDB URL")
	flag.StringVar(&cfg.InfluxToken, "influx-token", "", "InfluxDB authentication token")
	flag.StringVar(&cfg.InfluxOrg, "influx-org", "default", "InfluxDB organization")
	flag.StringVar(&cfg.InfluxBucket, "influx-bucket", "k8s", "InfluxDB bucket")
	flag.StringVar(&cfg.KubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flag.DurationVar(&cfg.PollInterval, "interval", 30*time.Second, "Metrics collection interval")
	flag.StringVar(&cfg.ListenAddr, "listen-addr", ":8080", "Web server listen address")
	flag.BoolVar(&cfg.TestMode, "test-mode", false, "Start in test mode with mock data for first metric collection")
	flag.Parse()

	if cfg.InfluxToken == "" {
		log.Fatal("InfluxDB token is required. Please provide it using --influx-token flag")
	}

	return cfg
}
