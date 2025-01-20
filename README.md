# tinykmetrics

Tiny kubernetes metrics collector for influxdb.

## Start influxdb

```bash
podman run -p 8086:8086 \
  -e DOCKER_INFLUXDB_INIT_MODE=setup \
  -e DOCKER_INFLUXDB_INIT_USERNAME=admin \
  -e DOCKER_INFLUXDB_INIT_PASSWORD=adminadmin \
  -e DOCKER_INFLUXDB_INIT_ORG=myorg \
  -e DOCKER_INFLUXDB_INIT_BUCKET=k8s \
  -e DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=my-super-secret-auth-token \
  influxdb:2.7.1
```

## Run tinykmetrics

```bash
go run cmd/tinykmetrics/main.go --influx-url=http://localhost:8086 \
         --influx-token=my-super-secret-auth-token \
         --influx-org=myorg \
         --influx-bucket=k8s \
         --kubeconfig=/Users/$USER/.kube/config
```

## Kubernetes

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tinykmetrics
  namespace: monitoring
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: metrics-reader
rules:
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: tinykmetrics-metrics-reader
subjects:
- kind: ServiceAccount
  name: tinykmetrics
  namespace: monitoring
roleRef:
  kind: ClusterRole
  name: metrics-reader
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tinykmetrics
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tinykmetrics
  template:
    metadata:
      labels:
        app: tinykmetrics
    spec:
      serviceAccountName: tinykmetrics
      containers:
      - name: tinykmetrics
        image: tinykmetrics:latest
        args:
        - --influx-url=http://influxdb:8086
        - --influx-token=your-token-here
        - --influx-org=your-org
        - --influx-bucket=k8s
        - --interval=30s
      livenessProbe:
        httpGet:
          path: /status
          port: 8080
        initialDelaySeconds: 3
        periodSeconds: 3
      readinessProbe:
        httpGet:
          path: /ready
          port: 8080
        initialDelaySeconds: 5
        periodSeconds: 5
```
