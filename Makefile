NETWORK_NAME = testnetwork
APP_CONTAINER = test-tinykmetrics
DB_CONTAINER = test-influxdb
AUTH_TOKEN = my-super-secret-auth-token
ORG = myorg
BUCKET = k8s

test-deps:
	@which podman >/dev/null 2>&1 || (echo "❌ podman is required but not installed. Aborting." && exit 1)
	@which curl >/dev/null 2>&1 || (echo "❌ curl is required but not installed. Aborting." && exit 1)
	@which jq >/dev/null 2>&1 || (echo "❌ jq is required but not installed. Aborting." && exit 1)

test: test-deps
	@echo "ℹ️ Creating podman network..."
	podman network create $(NETWORK_NAME) || true

	@echo "ℹ️ Starting InfluxDB container..."
	podman run -d --name $(DB_CONTAINER) \
		--network $(NETWORK_NAME) \
		-p 8086:8086 \
		-e DOCKER_INFLUXDB_INIT_MODE=setup \
		-e DOCKER_INFLUXDB_INIT_USERNAME=admin \
		-e DOCKER_INFLUXDB_INIT_PASSWORD=adminadmin \
		-e DOCKER_INFLUXDB_INIT_ORG=$(ORG) \
		-e DOCKER_INFLUXDB_INIT_BUCKET=$(BUCKET) \
		-e DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=$(AUTH_TOKEN) \
		influxdb:latest

	@echo "ℹ️ Waiting for InfluxDB to be ready..."
	sleep 5

	@echo "ℹ️ Building application container..."
	podman build -t $(APP_CONTAINER) -f Dockerfile .

	@echo "ℹ️ Running application container..."
	podman run -d --name $(APP_CONTAINER) \
		--network $(NETWORK_NAME) \
		-p 8080:8080 \
		$(APP_CONTAINER) \
		/tinykmetrics \
		-influx-url=http://$(DB_CONTAINER):8086 \
		-influx-token=$(AUTH_TOKEN) \
		-influx-org=$(ORG) \
		-influx-bucket=$(BUCKET) \
		--test-mode \
		-interval=30s

	@echo "✅ Test environment is ready!"

	@echo "ℹ️ Running integration tests..."
	@echo "ℹ️ Waiting for application to be ready..."
	@MAX_RETRIES=30; \
	RETRY_COUNT=0; \
	while ! curl -s "http://localhost:8080/ready" > /dev/null 2>&1; do \
		if [ $$RETRY_COUNT -ge $$MAX_RETRIES ]; then \
			echo "❌ Timeout waiting for application to start"; \
			exit 1; \
		fi; \
		echo "ℹ️ Waiting... ($$(($$RETRY_COUNT + 1))/$$MAX_RETRIES)"; \
		sleep 2; \
		RETRY_COUNT=$$(($$RETRY_COUNT + 1)); \
	done

	@echo "✅ Application is ready"
	@echo "ℹ️ Running tests..."

	@echo "ℹ️ Test 1: Get metrics for a specific pod"
	@METRICS_RESPONSE=$$(curl -s -X POST "http://localhost:8080/api/metrics" \
		-H "Content-Type: application/json" \
		-d '{"start":"5m","namespace":"","pod":"postgres-1"}'); \
	if echo "$$METRICS_RESPONSE" | jq -e 'length > 0 and .[0].container == "postgres" and .[0].field == "cpu_usage" and .[0].namespace == "database" and .[0].pod == "postgres-1"' > /dev/null; then \
		echo "✅ Test 1 passed: Metrics response is valid"; \
	else \
		echo "❌ Test 1 failed: Metrics response does not match expected format"; \
		echo "Expected: Container=postgres, Field=cpu_usage, Namespace=database, Pod=postgres-1"; \
		echo "Actual: $$(echo "$$METRICS_RESPONSE" | jq '.[0]')"; \
		exit 1; \
	fi

	@echo "ℹ️ Test 2: Get list of namespaces"
	@NAMESPACES_RESPONSE=$$(curl -s "http://localhost:8080/api/namespaces"); \
	if echo "$$NAMESPACES_RESPONSE" | jq -e '.namespaces and (.namespaces | contains(["default", "kube-system", "monitoring", "database"]))' > /dev/null; then \
		echo "✅ Test 2 passed: Namespaces response is valid"; \
	else \
		echo "❌ Test 2 failed: Namespaces response does not match expected format"; \
		echo "Expected: Contains default, kube-system, monitoring, database"; \
		echo "Actual: $$NAMESPACES_RESPONSE"; \
		exit 1; \
	fi

	@echo "ℹ️ Test 3: Get pods for a specific namespace"
	@PODS_RESPONSE=$$(curl -s "http://localhost:8080/api/pods?namespace=default"); \
	if echo "$$PODS_RESPONSE" | jq -e '.pods and (.pods[0].name == "web-app-1" and .pods[0].namespace == "default")' > /dev/null; then \
		echo "✅ Test 3 passed: Pods response is valid"; \
	else \
		echo "❌ Test 3 failed: Pods response does not match expected format"; \
		echo "Expected: Pod with name=web-app-1, namespace=default"; \
		echo "Actual: $$PODS_RESPONSE"; \
		exit 1; \
	fi

	@echo "✅ All tests passed!"

clean:
	@echo "ℹ️ Cleaning up containers and volumes..."
	podman stop $(APP_CONTAINER) $(DB_CONTAINER) || true
	podman rm -v $(APP_CONTAINER) $(DB_CONTAINER) || true
	podman network rm $(NETWORK_NAME) || true