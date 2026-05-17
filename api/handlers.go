package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/gorilla/mux"
)

type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// containerFirstName returns the first name of a container (without leading "/").
// Falls back to the short container ID if no names are available.
func containerFirstName(c types.Container) string {
	if len(c.Names) == 0 {
		if len(c.ID) >= 12 {
			return c.ID[:12]
		}
		return c.ID
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// findComposeFilePath returns the path to the compose file in the given directory,
// trying all common filenames. Returns empty string if none found.
func findComposeFilePath(dir string) string {
	names := []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
	for _, name := range names {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
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

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	ctx := context.Background()

	// Check if container uses TTY (TTY containers don't use Docker's multiplexed stream format)
	inspect, err := s.docker.ContainerInspect(ctx, id)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

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

	var logContent string
	if inspect.Config.Tty {
		// TTY containers: raw stream, no multiplexed headers
		data, readErr := io.ReadAll(logs)
		if readErr != nil && readErr != io.EOF {
			s.sendError(w, http.StatusInternalServerError, readErr.Error())
			return
		}
		logContent = string(data)
	} else {
		// Non-TTY containers: Docker multiplexed stream (stdout + stderr with 8-byte headers)
		var stdout, stderr bytes.Buffer
		stdcopy.StdCopy(&stdout, &stderr, logs)
		logContent = stdout.String() + stderr.String()
	}

	if logContent == "" {
		logContent = "No logs available"
	}

	// Sanitize: replace invalid UTF-8 sequences and non-printable control characters
	// (except newline/tab) to avoid corrupting the JSON response
	logContent = strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return '?'
		}
		if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
			return -1 // drop the character
		}
		return r
	}, strings.ToValidUTF8(logContent, "?"))

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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    metrics,
	})
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    metrics,
	})
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
		switch info.Name() {
		case "docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml":
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

	composeFile := findComposeFilePath(req.Path)
	if composeFile == "" {
		s.sendError(w, http.StatusBadRequest, "No compose file found in "+req.Path)
		return
	}

	var cmd *exec.Cmd
	switch req.Action {
	case "up":
		cmd = exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
	case "down":
		cmd = exec.Command("docker", "compose", "-f", composeFile, "down")
	case "restart":
		cmd = exec.Command("docker", "compose", "-f", composeFile, "restart")
	case "pull":
		cmd = exec.Command("docker", "compose", "-f", composeFile, "pull")
	case "ps":
		cmd = exec.Command("docker", "compose", "-f", composeFile, "ps", "--format", "json")
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

// Check for container image updates
func (s *Server) handleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	log.Println("Check updates requested")
	ctx := context.Background()
	containers, err := s.docker.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		log.Printf("Error listing containers: %v", err)
		s.sendError(w, http.StatusInternalServerError, "Failed to list containers")
		return
	}

	log.Printf("Found %d running containers", len(containers))

	type UpdateInfo struct {
		ContainerName string `json:"container_name"`
		CurrentImage  string `json:"current_image"`
		HasUpdate     bool   `json:"has_update"`
	}

	updates := []UpdateInfo{}

	// Group containers by image
	imageMap := make(map[string][]string)
	for _, c := range containers {
		name := containerFirstName(c)
		inspect, err := s.docker.ContainerInspect(ctx, c.ID)
		if err != nil {
			continue
		}
		imageTag := inspect.Config.Image
		imageMap[imageTag] = append(imageMap[imageTag], name)
	}

	log.Printf("Checking %d unique images", len(imageMap))

	// Check each unique image by inspecting remote
	for imageTag, containerNames := range imageMap {
		// Get local image digest
		inspect, err := s.docker.ContainerInspect(ctx, containerNames[0])
		if err != nil {
			continue
		}
		localDigest := inspect.Image

		// Inspect remote image (no pull, just metadata)
		distInspect, err := s.docker.DistributionInspect(ctx, imageTag, "")
		hasUpdate := false

		if err == nil {
			remoteDigest := distInspect.Descriptor.Digest.String()
			// Compare digests
			if localDigest != "" && remoteDigest != "" && !strings.Contains(localDigest, remoteDigest) {
				hasUpdate = true
				log.Printf("Update available for %s: local=%s remote=%s", imageTag, localDigest[:20], remoteDigest[:20])
			}
		} else {
			log.Printf("Could not check remote for %s: %v", imageTag, err)
		}

		for _, name := range containerNames {
			updates = append(updates, UpdateInfo{
				ContainerName: name,
				CurrentImage:  imageTag,
				HasUpdate:     hasUpdate,
			})
		}
	}

	log.Printf("Check complete. Found %d containers", len(updates))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    updates,
	})
}
