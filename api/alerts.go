package api

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Alert struct {
	ID            string    `json:"id"`
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	Type          string    `json:"type"` // "cpu" or "memory"
	Threshold     float64   `json:"threshold"`
	CurrentValue  float64   `json:"current_value"`
	Triggered     bool      `json:"triggered"`
	TriggeredAt   time.Time `json:"triggered_at"`
	Message       string    `json:"message"`
}

type AlertConfig struct {
	ContainerID   string  `json:"container_id"`
	CPUThreshold  float64 `json:"cpu_threshold"`  // 0-100
	MemThreshold  float64 `json:"mem_threshold"`  // 0-100
	Enabled       bool    `json:"enabled"`
}

type AlertStore struct {
	configs     map[string]AlertConfig // containerID -> config
	history     []Alert                // last 100 alerts
	mu          sync.RWMutex
	file        string
	onAlert     func(Alert) // callback for notifications
}

func NewAlertStore(file string, onAlert func(Alert)) *AlertStore {
	store := &AlertStore{
		configs: make(map[string]AlertConfig),
		history: make([]Alert, 0),
		file:    file,
		onAlert: onAlert,
	}
	store.load()
	return store
}

func (a *AlertStore) SetConfig(config AlertConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configs[config.ContainerID] = config
	a.save()
}

func (a *AlertStore) GetConfig(containerID string) (AlertConfig, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	config, ok := a.configs[containerID]
	return config, ok
}

func (a *AlertStore) GetAllConfigs() []AlertConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	configs := make([]AlertConfig, 0, len(a.configs))
	for _, c := range a.configs {
		configs = append(configs, c)
	}
	return configs
}

func (a *AlertStore) DeleteConfig(containerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.configs, containerID)
	a.save()
}

func (a *AlertStore) CheckMetric(containerID, containerName string, cpuPercent, memPercent float64) {
	config, ok := a.GetConfig(containerID)
	if !ok || !config.Enabled {
		return
	}

	// Check CPU
	if config.CPUThreshold > 0 && cpuPercent > config.CPUThreshold {
		alert := Alert{
			ID:            time.Now().Format("20060102150405") + "-cpu-" + containerID[:12],
			ContainerID:   containerID,
			ContainerName: containerName,
			Type:          "cpu",
			Threshold:     config.CPUThreshold,
			CurrentValue:  cpuPercent,
			Triggered:     true,
			TriggeredAt:   time.Now(),
			Message:       "CPU usage exceeded threshold",
		}
		a.addAlert(alert)
		if a.onAlert != nil {
			a.onAlert(alert)
		}
	}

	// Check Memory
	if config.MemThreshold > 0 && memPercent > config.MemThreshold {
		alert := Alert{
			ID:            time.Now().Format("20060102150405") + "-mem-" + containerID[:12],
			ContainerID:   containerID,
			ContainerName: containerName,
			Type:          "memory",
			Threshold:     config.MemThreshold,
			CurrentValue:  memPercent,
			Triggered:     true,
			TriggeredAt:   time.Now(),
			Message:       "Memory usage exceeded threshold",
		}
		a.addAlert(alert)
		if a.onAlert != nil {
			a.onAlert(alert)
		}
	}
}

func (a *AlertStore) addAlert(alert Alert) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = append(a.history, alert)
	if len(a.history) > 100 {
		a.history = a.history[len(a.history)-100:]
	}
}

func (a *AlertStore) GetHistory(limit int) []Alert {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if limit <= 0 || limit > len(a.history) {
		limit = len(a.history)
	}
	start := len(a.history) - limit
	if start < 0 {
		start = 0
	}
	result := make([]Alert, limit)
	copy(result, a.history[start:])
	// Reverse to show newest first
	for i := 0; i < len(result)/2; i++ {
		result[i], result[len(result)-1-i] = result[len(result)-1-i], result[i]
	}
	return result
}

func (a *AlertStore) load() {
	data, err := os.ReadFile(a.file)
	if err != nil {
		return
	}
	var stored struct {
		Configs map[string]AlertConfig `json:"configs"`
		History []Alert                `json:"history"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	a.configs = stored.Configs
	a.history = stored.History
}

func (a *AlertStore) save() {
	data := struct {
		Configs map[string]AlertConfig `json:"configs"`
		History []Alert                `json:"history"`
	}{
		Configs: a.configs,
		History: a.history,
	}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(a.file, bytes, 0644)
}
