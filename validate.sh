#!/bin/bash
set -e

echo "🔍 Validando código..."

echo "1️⃣ Verificando sintaxis Go..."
docker run --rm -v $(pwd):/app -w /app golang:alpine sh -c "go fmt ./... && echo '✅ Formato correcto'"

echo ""
echo "2️⃣ Compilando..."
docker run --rm -v $(pwd):/app -w /app golang:alpine sh -c "go build -o /tmp/botainer main.go && echo '✅ Compilación exitosa'"

echo ""
echo "3️⃣ Verificando imports no usados..."
docker run --rm -v $(pwd):/app -w /app golang:alpine sh -c "go mod tidy && echo '✅ Dependencias limpias'"

echo ""
echo "✅ Todas las validaciones pasaron. Listo para deploy."
