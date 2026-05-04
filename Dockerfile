FROM golang:alpine AS builder
WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -o botainer main.go

FROM alpine:latest
RUN apk add --no-cache docker-cli docker-cli-compose
COPY --from=builder /build/botainer /usr/local/bin/botainer
COPY locale /app/locale
WORKDIR /workspace
CMD ["botainer"]
