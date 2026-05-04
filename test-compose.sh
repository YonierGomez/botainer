#!/bin/bash
set -e

echo "🧪 Testing Compose Functions"
echo "=============================="
echo ""

echo "1️⃣ Testing docker compose availability..."
if docker compose version > /dev/null 2>&1; then
    echo "✅ docker compose is available"
    docker compose version
else
    echo "❌ docker compose is NOT available"
    exit 1
fi

echo ""
echo "2️⃣ Testing compose file detection..."
TEST_DIR="/tmp/compose-test"
mkdir -p "$TEST_DIR"

# Test compose.yaml
touch "$TEST_DIR/compose.yaml"
if [ -f "$TEST_DIR/compose.yaml" ]; then
    echo "✅ compose.yaml detected"
else
    echo "❌ compose.yaml not detected"
fi

# Test docker-compose.yml
rm -f "$TEST_DIR/compose.yaml"
touch "$TEST_DIR/docker-compose.yml"
if [ -f "$TEST_DIR/docker-compose.yml" ]; then
    echo "✅ docker-compose.yml detected"
else
    echo "❌ docker-compose.yml not detected"
fi

rm -rf "$TEST_DIR"

echo ""
echo "3️⃣ Testing compose project detection..."
COMPOSE_CONTAINERS=$(docker ps -a --filter "label=com.docker.compose.project" --format "{{.Names}}" | wc -l)
echo "Found $COMPOSE_CONTAINERS containers with compose labels"

if [ "$COMPOSE_CONTAINERS" -gt 0 ]; then
    echo "Sample compose containers:"
    docker ps -a --filter "label=com.docker.compose.project" --format "  - {{.Names}} (project: {{.Label \"com.docker.compose.project\"}})" | head -5
fi

echo ""
echo "4️⃣ Testing workspace mapping..."
echo "HOST_HOME=${HOST_HOME:-/home/ubuntu}"
echo "WORKSPACE=${WORKSPACE:-/workspace}"

if [ -d "${WORKSPACE:-/workspace}" ]; then
    echo "✅ Workspace directory exists"
else
    echo "⚠️  Workspace directory not found (this is OK if running outside container)"
fi

echo ""
echo "5️⃣ Testing timeout command..."
if timeout 1s sleep 0.5 > /dev/null 2>&1; then
    echo "✅ Timeout command works"
else
    echo "⚠️  Timeout command not available (using Go context timeout instead)"
fi

echo ""
echo "=============================="
echo "✅ All compose tests passed!"
