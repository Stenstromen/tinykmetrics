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
	chmod +x ./integration_test.sh
	./integration_test.sh

clean:
	@echo "ℹ️ Cleaning up containers and volumes..."
	podman stop $(APP_CONTAINER) $(DB_CONTAINER) || true
	podman rm -v $(APP_CONTAINER) $(DB_CONTAINER) || true
	podman network rm $(NETWORK_NAME) || true