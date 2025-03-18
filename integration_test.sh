#!/bin/bash
set -e

APP_CONTAINER="test-tinykmetrics"
APP_IP="localhost"
DB_CONTAINER="test-influxdb"

# Wait for the application to be ready
echo "ℹ️ Waiting for application to be ready..."
MAX_RETRIES=30
RETRY_COUNT=0
while ! curl -s "http://$APP_IP:8080/ready" > /dev/null 2>&1; do
    if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
        fail "Timeout waiting for application to start"
    fi
    echo "ℹ️ Waiting... ($(($RETRY_COUNT + 1))/$MAX_RETRIES)"
    sleep 2
    RETRY_COUNT=$((RETRY_COUNT + 1))
done

echo "✅ Application is ready"

echo "ℹ️ Running tests..."

# Test 1: Get metrics for a specific pod
echo "ℹ️ Test 1: Get metrics for a specific pod"
METRICS_RESPONSE=$(curl -s -X POST "http://$APP_IP:8080/api/metrics" \
  -H "Content-Type: application/json" \
  -d '{"start":"5m","namespace":"","pod":"postgres-1"}')

# Validate the metrics response contains expected fields
if echo "$METRICS_RESPONSE" | jq -e 'length > 0 and .[0].container == "postgres" and .[0].field == "cpu_usage" and .[0].namespace == "database" and .[0].pod == "postgres-1"' > /dev/null; then
  echo "✅ Test 1 passed: Metrics response is valid"
else
  echo "❌ Test 1 failed: Metrics response does not match expected format"
  echo "Expected: Container=postgres, Field=cpu_usage, Namespace=database, Pod=postgres-1"
  echo "Actual: $(echo "$METRICS_RESPONSE" | jq '.[0]')"
  exit 1
fi

# Test 2: Get list of namespaces
echo "ℹ️ Test 2: Get list of namespaces"
NAMESPACES_RESPONSE=$(curl -s "http://$APP_IP:8080/api/namespaces")

# Validate the namespaces response
if echo "$NAMESPACES_RESPONSE" | jq -e '.namespaces and (.namespaces | contains(["default", "kube-system", "monitoring", "database"]))' > /dev/null; then
  echo "✅ Test 2 passed: Namespaces response is valid"
else
  echo "❌ Test 2 failed: Namespaces response does not match expected format"
  echo "Expected: Contains default, kube-system, monitoring, database"
  echo "Actual: $NAMESPACES_RESPONSE"
  exit 1
fi

# Test 3: Get pods for a specific namespace
echo "ℹ️ Test 3: Get pods for a specific namespace"
PODS_RESPONSE=$(curl -s "http://$APP_IP:8080/api/pods?namespace=default")

# Validate the pods response
if echo "$PODS_RESPONSE" | jq -e '.pods and (.pods[0].name == "web-app-1" and .pods[0].namespace == "default")' > /dev/null; then
  echo "✅ Test 3 passed: Pods response is valid"
else
  echo "❌ Test 3 failed: Pods response does not match expected format"
  echo "Expected: Pod with name=web-app-1, namespace=default"
  echo "Actual: $PODS_RESPONSE"
  exit 1
fi

echo "✅ All tests passed!"
exit 0

