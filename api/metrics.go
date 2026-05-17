package api

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type MetricPoint struct {
	Timestamp     int64   `json:"timestamp"`
	ContainerID   string  `json:"container_id"`
	ContainerName string  `json:"container_name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   float64 `json:"memory_usage"`
	MemoryLimit   float64 `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
}

type MetricsStore struct {
	mu      sync.RWMutex
	metrics []MetricPoint
	maxSize int
	file    string
}

func NewMetricsStore(file string, maxSize int) *MetricsStore {
	store := &MetricsStore{
		metrics: make([]MetricPoint, 0),
		maxSize: maxSize,
		file:    file,
	}
	store.load()
	return store
}

func (m *MetricsStore) Add(point MetricPoint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = append(m.metrics, point)

	// Keep only last maxSize points
	if len(m.metrics) > m.maxSize {
		m.metrics = m.metrics[len(m.metrics)-m.maxSize:]
	}

	m.save()
}

func (m *MetricsStore) GetRange(containerID string, start, end int64) []MetricPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MetricPoint, 0)
	for _, point := range m.metrics {
		if (containerID == "" || point.ContainerID == containerID) &&
			point.Timestamp >= start && point.Timestamp <= end {
			result = append(result, point)
		}
	}
	return result
}

func (m *MetricsStore) GetLast(containerID string, duration time.Duration) []MetricPoint {
	now := time.Now().Unix()
	start := now - int64(duration.Seconds())
	return m.GetRange(containerID, start, now)
}

func (m *MetricsStore) save() {
	data, err := json.Marshal(m.metrics)
	if err != nil {
		log.Printf("Error marshaling metrics: %v", err)
		return
	}

	if err := os.WriteFile(m.file, data, 0644); err != nil {
		log.Printf("Error saving metrics: %v", err)
	}
}

func (m *MetricsStore) load() {
	data, err := os.ReadFile(m.file)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error loading metrics: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, &m.metrics); err != nil {
		log.Printf("Error unmarshaling metrics: %v", err)
	}
}

// CollectMetrics collects metrics from all containers every interval
func CollectMetrics(dockerClient *client.Client, store *MetricsStore, alertStore *AlertStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: false})
		if err != nil {
			log.Printf("Error listing containers for metrics: %v", err)
			continue
		}

		for _, c := range containers {
			stats, err := dockerClient.ContainerStats(ctx, c.ID, false)
			if err != nil {
				continue
			}

			var v container.StatsResponse
			if err := json.NewDecoder(stats.Body).Decode(&v); err != nil {
				stats.Body.Close()
				continue
			}
			stats.Body.Close()

			// Calculate CPU
			cpuPercent := 0.0
			cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
			numCPUs := float64(len(v.CPUStats.CPUUsage.PercpuUsage))
			if numCPUs == 0 {
				numCPUs = 1
			}
			if systemDelta > 0 && cpuDelta > 0 {
				cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
			}

			// Calculate memory
			memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 / 1024
			memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 / 1024
			memPercent := 0.0
			if memLimit > 0 {
				memPercent = (memUsage / memLimit) * 100.0
			}

			point := MetricPoint{
				Timestamp:     time.Now().Unix(),
				ContainerID:   c.ID[:12],
				ContainerName: containerFirstName(c),
				CPUPercent:    cpuPercent,
				MemoryUsage:   memUsage,
				MemoryLimit:   memLimit,
				MemoryPercent: memPercent,
			}

			store.Add(point)

			// Check alerts
			if alertStore != nil {
			alertStore.CheckMetric(c.ID[:12], containerFirstName(c), cpuPercent, memPercent)
		}
		}
	}
}
