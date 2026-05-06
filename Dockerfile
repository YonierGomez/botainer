FROM golang:alpine AS builder
WORKDIR /build
COPY go.mod go.sum* ./
COPY main.go ./
COPY api/ ./api/
RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 go build -o botainer main.go

FROM alpine:latest
RUN apk add --no-cache docker-cli docker-cli-compose curl
RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin
COPY --from=builder /build/botainer /usr/local/bin/botainer
COPY locale /app/locale
WORKDIR /workspace
CMD ["botainer"]
