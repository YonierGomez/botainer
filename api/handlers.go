package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
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
	
	// Get tail parameter from query string
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	
	ctx := context.Background()
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	}
	
	logs, err := s.docker.ContainerLogs(ctx, id, options)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer logs.Close()
	
	// Read all logs into a string
	// Docker logs use a multiplexed stream format with 8-byte headers
	var logContent string
	buf := make([]byte, 8192)
	for {
		n, err := logs.Read(buf)
		if n > 0 {
			// Process the buffer, skipping Docker's 8-byte headers
			i := 0
			for i < n {
				if i+8 > n {
					break
				}
				// Read the 4-byte size from header (bytes 4-7)
				size := int(buf[i+4])<<24 | int(buf[i+5])<<16 | int(buf[i+6])<<8 | int(buf[i+7])
				i += 8
				if i+size > n {
					// Partial message, add what we have
					logContent += string(buf[i:n])
					break
				}
				logContent += string(buf[i : i+size])
				i += size
			}
		}
		if err != nil {
			break
		}
	}
	
	if logContent == "" {
		logContent = "No logs available"
	}
	
	// Return as JSON
	s.sendJSON(w, http.StatusOK, logContent)
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
	
	// Calculate memory in GB
	memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 / 1024
	memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 / 1024
	memPercent := 0.0
	if memLimit > 0 {
		memPercent = (memUsage / memLimit) * 100.0
	}
	
	result := map[string]interface{}{
		"cpu_percent":    cpuPercent,
		"memory_usage":   memUsage,
		"memory_limit":   memLimit,
		"memory_percent": memPercent,
	}
	
	s.sendJSON(w, http.StatusOK, result)
}

func (s *Server) handleContainerMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	// Get duration from query (default 1h)
	durationStr := r.URL.Query().Get("duration")
	duration := time.Hour
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}
	
	metrics := s.metricsStore.GetLast(id, duration)
	s.sendJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleAllMetrics(w http.ResponseWriter, r *http.Request) {
	// Get duration from query (default 1h)
	durationStr := r.URL.Query().Get("duration")
	duration := time.Hour
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}
	
	metrics := s.metricsStore.GetLast("", duration)
	s.sendJSON(w, http.StatusOK, metrics)
}

// Network handlers
func (s *Server) handleListNetworks(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	networks, err := s.docker.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get containers for each network
	result := make([]map[string]interface{}, 0)
	for _, net := range networks {
		containers := make([]map[string]string, 0)
		for id, endpoint := range net.Containers {
			containers = append(containers, map[string]string{
				"id":   id[:12],
				"name": endpoint.Name,
				"ipv4": endpoint.IPv4Address,
			})
		}
		
		result = append(result, map[string]interface{}{
			"id":         net.ID[:12],
			"name":       net.Name,
			"driver":     net.Driver,
			"scope":      net.Scope,
			"containers": containers,
		})
	}

	s.sendJSON(w, http.StatusOK, result)
}

// Container creation handler
func (s *Server) handleCreateContainer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string            `json:"name"`
		Image   string            `json:"image"`
		Env     []string          `json:"env"`
		Ports   map[string]string `json:"ports"`
		Volumes []string          `json:"volumes"`
		Network string            `json:"network"`
		Restart string            `json:"restart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx := context.Background()
	
	// Create container config
	config := &container.Config{
		Image: req.Image,
		Env:   req.Env,
	}
	
	// Host config with ports and volumes
	hostConfig := &container.HostConfig{
		PortBindings: make(map[nat.Port][]nat.PortBinding),
		Binds:        req.Volumes,
	}
	
	// Parse port mappings
	for containerPort, hostPort := range req.Ports {
		port := nat.Port(containerPort)
		hostConfig.PortBindings[port] = []nat.PortBinding{{HostPort: hostPort}}
	}
	
	// Set restart policy
	if req.Restart != "" {
		hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(req.Restart)}
	}
	
	// Network config
	networkConfig := &network.NetworkingConfig{}
	if req.Network != "" {
		networkConfig.EndpointsConfig = map[string]*network.EndpointSettings{
			req.Network: {},
		}
	}
	
	// Create container
	resp, err := s.docker.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, req.Name)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// Start container
	if err := s.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.sendJSON(w, http.StatusOK, map[string]string{
		"id":      resp.ID,
		"message": "Container created and started",
	})
}

// Docker Compose handlers
func (s *Server) handleListComposeProjects(w http.ResponseWriter, r *http.Request) {
	workspace := r.URL.Query().Get("workspace")
	if workspace == "" {
		workspace = "/workspace"
	}

	projects := make([]map[string]interface{}, 0)
	
	// Find compose files
	filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "docker-compose.yml" || info.Name() == "compose.yaml" {
			dir := filepath.Dir(path)
			projectName := filepath.Base(dir)
			
			projects = append(projects, map[string]interface{}{
				"name": projectName,
				"path": dir,
				"file": path,
			})
		}
		return nil
	})

	s.sendJSON(w, http.StatusOK, projects)
}

func (s *Server) handleComposeAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		Action string `json:"action"` // up, down, restart, pull, ps
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var cmd *exec.Cmd
	switch req.Action {
	case "up":
		cmd = exec.Command("docker", "compose", "-f", filepath.Join(req.Path, "compose.yaml"), "up", "-d")
	case "down":
		cmd = exec.Command("docker", "compose", "-f", filepath.Join(req.Path, "compose.yaml"), "down")
	case "restart":
		cmd = exec.Command("docker", "compose", "-f", filepath.Join(req.Path, "compose.yaml"), "restart")
	case "pull":
		cmd = exec.Command("docker", "compose", "-f", filepath.Join(req.Path, "compose.yaml"), "pull")
	case "ps":
		cmd = exec.Command("docker", "compose", "-f", filepath.Join(req.Path, "compose.yaml"), "ps", "--format", "json")
	default:
		s.sendError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, string(output))
		return
	}

	s.sendJSON(w, http.StatusOK, map[string]string{
		"output": string(output),
	})
}

// Bulk operations handler
func (s *Server) handleBulkAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContainerIDs []string `json:"container_ids"`
		Action       string   `json:"action"` // start, stop, restart, delete
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.ContainerIDs) == 0 {
		s.sendError(w, http.StatusBadRequest, "No containers specified")
		return
	}

	ctx := context.Background()
	results := make(map[string]interface{})
	
	for _, id := range req.ContainerIDs {
		var err error
		switch req.Action {
		case "start":
			err = s.docker.ContainerStart(ctx, id, container.StartOptions{})
		case "stop":
			timeout := 10
			err = s.docker.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
		case "restart":
			timeout := 10
			err = s.docker.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
		case "delete":
			err = s.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
		default:
			results[id] = "invalid action"
			continue
		}
		
		if err != nil {
			results[id] = err.Error()
		} else {
			results[id] = "success"
		}
	}

	s.sendJSON(w, http.StatusOK, results)
}

// Alert handlers
func (s *Server) handleGetAlertConfigs(w http.ResponseWriter, r *http.Request) {
	configs := s.alertStore.GetAllConfigs()
	s.sendJSON(w, http.StatusOK, configs)
}

func (s *Server) handleSetAlertConfig(w http.ResponseWriter, r *http.Request) {
	var config AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	s.alertStore.SetConfig(config)
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Alert config saved"})
}

func (s *Server) handleDeleteAlertConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]
	s.alertStore.DeleteConfig(containerID)
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Alert config deleted"})
}

func (s *Server) handleGetAlertHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	history := s.alertStore.GetHistory(limit)
	s.sendJSON(w, http.StatusOK, history)
}

func (s *Server) handleExportMetrics(w http.ResponseWriter, r *http.Request) {
	// Get parameters
	format := r.URL.Query().Get("format") // csv or json
	if format == "" {
		format = "json"
	}
	
	durationStr := r.URL.Query().Get("duration")
	duration := 24 * time.Hour
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}
	
	containerID := r.URL.Query().Get("container")
	metrics := s.metricsStore.GetLast(containerID, duration)
	
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=metrics.csv")
		
		// Write CSV header
		w.Write([]byte("timestamp,container_id,container_name,cpu_percent,memory_usage_gb,memory_limit_gb,memory_percent\n"))
		
		// Write data
		for _, m := range metrics {
			line := fmt.Sprintf("%d,%s,%s,%.2f,%.2f,%.2f,%.2f\n",
				m.Timestamp, m.ContainerID, m.ContainerName,
				m.CPUPercent, m.MemoryUsage, m.MemoryLimit, m.MemoryPercent)
			w.Write([]byte(line))
		}
	} else {
		s.sendJSON(w, http.StatusOK, metrics)
	}
}

// User management handlers
func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	users := s.userStore.GetUsers()
	s.sendJSON(w, http.StatusOK, users)
}

func (s *Server) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]
	
	var req struct {
		Role Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	
	if req.Role != RoleAdmin && req.Role != RoleOperator && req.Role != RoleViewer {
		s.sendError(w, http.StatusBadRequest, "Invalid role")
		return
	}
	
	if err := s.userStore.UpdateRole(userID, req.Role); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to update role")
		return
	}
	
	s.userStore.LogAction(userID, "", "update_role", "user:"+userID, true, string(req.Role))
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Role updated"})
}

func (s *Server) handleGetAuditLog(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	
	logs := s.userStore.GetAuditLog(limit)
	s.sendJSON(w, http.StatusOK, logs)
}

// Template library handlers
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	publicOnly := r.URL.Query().Get("public") == "true"
	userID := r.URL.Query().Get("user_id")
	templates := s.templateStore.List(userID, publicOnly)
	s.sendJSON(w, http.StatusOK, templates)
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var template Template
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		s.sendError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	
	if template.Name == "" || template.Image == "" {
		s.sendError(w, http.StatusBadRequest, "Name and image are required")
		return
	}
	
	if err := s.templateStore.Create(&template); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to create template")
		return
	}
	
	s.sendJSON(w, http.StatusCreated, template)
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	template, exists := s.templateStore.Get(id)
	if !exists {
		s.sendError(w, http.StatusNotFound, "Template not found")
		return
	}
	
	s.sendJSON(w, http.StatusOK, template)
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	userID := r.URL.Query().Get("user_id")
	
	if err := s.templateStore.Delete(id, userID); err != nil {
		s.sendError(w, http.StatusInternalServerError, "Failed to delete template")
		return
	}
	
	s.sendJSON(w, http.StatusOK, map[string]string{"message": "Template deleted"})
}

func (s *Server) handleDeployTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	
	template, exists := s.templateStore.Get(id)
	if !exists {
		s.sendError(w, http.StatusNotFound, "Template not found")
		return
	}
	
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		s.sendError(w, http.StatusBadRequest, "Container name required")
		return
	}
	
	ctx := context.Background()
	
	// Parse ports
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for _, portStr := range template.Ports {
		parts := strings.Split(portStr, ":")
		if len(parts) == 2 {
			containerPort := nat.Port(parts[1] + "/tcp")
			portBindings[containerPort] = []nat.PortBinding{{HostPort: parts[0]}}
			exposedPorts[containerPort] = struct{}{}
		}
	}
	
	// Parse volumes
	binds := template.Volumes
	
	// Parse env
	env := make([]string, 0, len(template.Env))
	for k, v := range template.Env {
		env = append(env, k+"="+v)
	}
	
	// Create container
	resp, err := s.docker.ContainerCreate(ctx,
		&container.Config{
			Image:        template.Image,
			Env:          env,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			PortBindings:  portBindings,
			Binds:         binds,
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(template.RestartPolicy)},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				template.Network: {},
			},
		},
		nil,
		req.Name,
	)
	
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create container: %v", err))
		return
	}
	
	// Start container
	if err := s.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		s.sendError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to start container: %v", err))
		return
	}
	
	s.templateStore.IncrementUsage(id)
	s.sendJSON(w, http.StatusCreated, map[string]string{"id": resp.ID, "message": "Container deployed from template"})
}
