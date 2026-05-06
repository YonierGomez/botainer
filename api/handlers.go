package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gorilla/mux"
)

type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

func (s *Server) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success:   status < 400,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

func (s *Server) sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success:   false,
		Error:     message,
		Timestamp: time.Now().Unix(),
	})
}

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	containers, err := s.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, containers)
}

func (s *Server) handleGetContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	inspect, err := s.docker.ContainerInspect(ctx, id)
	if err != nil {
		s.sendError(w, http.StatusNotFound, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, inspect)
}

func (s *Server) handleStartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	if err := s.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Container started"})
}

func (s *Server) handleStopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	timeout := 10
	if err := s.docker.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Container stopped"})
}

func (s *Server) handleRestartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	timeout := 10
	if err := s.docker.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout}); err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Container restarted"})
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "100",
	}
	
	logs, err := s.docker.ContainerLogs(ctx, id, options)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer logs.Close()
	
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	// Copy logs to response
	buf := make([]byte, 8192)
	for {
		n, err := logs.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	info, err := s.docker.Info(ctx)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sendJSON(w, http.StatusOK, info)
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	ctx := context.Background()
	stats, err := s.docker.ContainerStats(ctx, id, false)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stats.Body.Close()
	
	var v container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&v); err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// Calculate CPU percentage
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	
	// Calculate memory
	memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 // MB
	memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 // MB
	memPercent := 0.0
	if memLimit > 0 {
		memPercent = (float64(v.MemoryStats.Usage) / float64(v.MemoryStats.Limit)) * 100.0
	}
	
	result := map[string]interface{}{
		"cpu_percent":    cpuPercent,
		"memory_usage":   memUsage,
		"memory_limit":   memLimit,
		"memory_percent": memPercent,
	}
	
	s.sendJSON(w, http.StatusOK, result)
}
