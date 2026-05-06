package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/YonierGomez/botainer/api"
	semver "github.com/Masterminds/semver/v3"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	botVersion     = "2.0.1"                      // Mini App: Added stats, search, filters, logs viewer
	newsChannelURL = "https://t.me/botainer_news" // Canal de novedades
	configFile     = "/data/config.json"          // Persistence file
)

var (
	bot                     *tgbotapi.BotAPI
	cli                     *client.Client
	notifyChatID            int64
	allowedUsers            []int64
	favorites               = make(map[int64][]string)
	commandHistory          = make(map[int64][]string)
	userState               = make(map[int64]string)
	createData              = make(map[int64]map[string]string)
	autoUpdateContainers    = make(map[string]bool)
	trackedImages           = make(map[string]string) // image:tag -> last known digest
	trackedCharts           = make(map[string]ChartInfo) // repo/chart -> chart info
	rollbackHistory         = make(map[string][]RollbackEntry) // container -> history (max 5)
	templates               = make(map[string]ContainerTemplate)
	maintenanceMode         bool
	maintenancePaused       []string // containers paused by maintenance mode
	
	// Phase 1: Alerts & Monitoring
	resourceAlerts          = make(map[string]ResourceAlert) // container -> alert config
	healthChecks            = make(map[string]HealthCheck)   // container -> health check config
	reportSchedule          = "daily"                         // daily, weekly, or disabled
	lastReportTime          time.Time
	
	// Phase 3: Security & Audit
	auditLog                []AuditEntry
	webhooks                = make(map[string]Webhook) // name -> webhook config
	updatePolicies          = make(map[string]UpdatePolicy) // container -> policy
	
	// Phase 4: Networking & Registry
	registries              = make(map[string]Registry) // name -> registry config
	criticalContainers      = map[string]bool{"botainer": true} // containers to never pause
	checkUpdatesInterval    = 6 * time.Hour
	enableAutoCheck         = true
	enableStartupNotif      = true
	configMutex             sync.Mutex
	language                = "es" // Default language
	translations            = make(map[string]string)
	containerIcons          = map[string]string{
		"botainer": "👑",
		"postgres": "🐘", "mysql": "🐬", "mariadb": "🐬", "mongo": "🍃",
		"redis": "⚡", "nginx": "🌐", "apache": "🪶", "node": "💚",
		"python": "🐍", "php": "🐘", "java": "☕", "golang": "🐹",
		"nextcloud": "☁️", "radarr": "🎬", "sonarr": "📺", "plex": "🎬",
		"jellyfin": "🎞️", "emby": "📺", "heimdall": "🏠", "homarr": "🏠",
		"wireguard": "🔒", "pihole": "🛡️", "adguard": "🛡️", "traefik": "🔀",
		"portainer": "🐳", "watchtower": "🗼", "grafana": "📊", "prometheus": "📈",
	}
)

// ChartInfo stores Helm chart tracking information
type ChartInfo struct {
	Version    string   `json:"version"`
	AppVersion string   `json:"appVersion"`
	Repo       string   `json:"repo"`
	Images     []string `json:"images"`
}

// Config structure for persistence
type Config struct {
	AutoUpdateContainers map[string]bool              `json:"autoUpdateContainers"`
	TrackedImages        map[string]string            `json:"trackedImages"` // image:tag -> digest
	TrackedCharts        map[string]ChartInfo         `json:"trackedCharts"` // repo/chart -> chart info
	LastCheck            time.Time                    `json:"lastCheck"`
	RollbackHistory      map[string][]RollbackEntry   `json:"rollbackHistory"`  // container -> history
	Templates            map[string]ContainerTemplate `json:"templates"`
	MaintenanceMode      bool                         `json:"maintenanceMode"`
	MaintenancePaused    []string                     `json:"maintenancePaused"` // containers paused by maintenance
	
	// Phase 1
	ResourceAlerts map[string]ResourceAlert `json:"resourceAlerts"`
	HealthChecks   map[string]HealthCheck   `json:"healthChecks"`
	ReportSchedule string                   `json:"reportSchedule"`
	LastReportTime time.Time                `json:"lastReportTime"`
	
	// Phase 3
	AuditLog       []AuditEntry             `json:"auditLog"`
	Webhooks       map[string]Webhook       `json:"webhooks"`
	UpdatePolicies map[string]UpdatePolicy  `json:"updatePolicies"`
	
	// Phase 4
	Registries map[string]Registry `json:"registries"`
}

// RollbackEntry stores a previous image for a container
type RollbackEntry struct {
	Image     string    `json:"image"`
	ImageID   string    `json:"imageId"`
	Timestamp time.Time `json:"timestamp"`
}

// ContainerTemplate stores a container configuration for reuse
type ContainerTemplate struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Cmd         []string          `json:"cmd,omitempty"`
	Env         []string          `json:"env,omitempty"`
	Ports       map[string]string `json:"ports,omitempty"` // hostPort -> containerPort
	Volumes     []string          `json:"volumes,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	NetworkMode string            `json:"networkMode,omitempty"`
	RestartPolicy string          `json:"restartPolicy,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// Phase 1: Alerts & Monitoring types
type ResourceAlert struct {
	CPUThreshold  float64 `json:"cpuThreshold"`  // percentage
	RAMThreshold  float64 `json:"ramThreshold"`  // percentage
	DiskThreshold float64 `json:"diskThreshold"` // percentage
	Enabled       bool    `json:"enabled"`
}

type HealthCheck struct {
	Type     string `json:"type"`     // http, tcp
	Target   string `json:"target"`   // URL or host:port
	Interval int    `json:"interval"` // seconds
	Enabled  bool   `json:"enabled"`
}

// Phase 3: Security & Audit types
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	UserID    int64     `json:"userId"`
	Command   string    `json:"command"`
	Target    string    `json:"target"`
	Success   bool      `json:"success"`
}

type Webhook struct {
	URL     string            `json:"url"`
	Events  []string          `json:"events"` // container_start, container_stop, update, etc
	Headers map[string]string `json:"headers,omitempty"`
	Enabled bool              `json:"enabled"`
}

type UpdatePolicy struct {
	Schedule      string `json:"schedule"`      // cron format or "immediate"
	MinFreeRAM    int    `json:"minFreeRam"`    // MB
	MinFreeDisk   int    `json:"minFreeDisk"`   // GB
	AutoApprove   bool   `json:"autoApprove"`   // auto-update without confirmation
	Enabled       bool   `json:"enabled"`
}

// Phase 4: Networking & Registry types
type Registry struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  bool   `json:"enabled"`
}

// ArtifactHubPackage represents a package from Artifact Hub API
type ArtifactHubPackage struct {
	Version     string `json:"version"`
	AppVersion  string `json:"app_version"`
	Repository  struct {
		Name string `json:"name"`
	} `json:"repository"`
	ContainersImages []struct {
		Image string `json:"image"`
	} `json:"containers_images"`
}

// Load configuration from file
func loadConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := os.ReadFile(configFile)
	if err != nil {
		return // File doesn't exist yet, use defaults
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("Error loading config: %v", err)
		return
	}

	autoUpdateContainers = cfg.AutoUpdateContainers
	if autoUpdateContainers == nil {
		autoUpdateContainers = make(map[string]bool)
	}
	trackedImages = cfg.TrackedImages
	if trackedImages == nil {
		trackedImages = make(map[string]string)
	}
	trackedCharts = cfg.TrackedCharts
	if trackedCharts == nil {
		trackedCharts = make(map[string]ChartInfo)
	}
	rollbackHistory = cfg.RollbackHistory
	if rollbackHistory == nil {
		rollbackHistory = make(map[string][]RollbackEntry)
	}
	templates = cfg.Templates
	if templates == nil {
		templates = make(map[string]ContainerTemplate)
	}
	maintenanceMode = cfg.MaintenanceMode
	maintenancePaused = cfg.MaintenancePaused
	
	// Phase 1
	resourceAlerts = cfg.ResourceAlerts
	if resourceAlerts == nil {
		resourceAlerts = make(map[string]ResourceAlert)
	}
	healthChecks = cfg.HealthChecks
	if healthChecks == nil {
		healthChecks = make(map[string]HealthCheck)
	}
	reportSchedule = cfg.ReportSchedule
	if reportSchedule == "" {
		reportSchedule = "daily"
	}
	lastReportTime = cfg.LastReportTime
	
	// Phase 3
	auditLog = cfg.AuditLog
	if auditLog == nil {
		auditLog = []AuditEntry{}
	}
	webhooks = cfg.Webhooks
	if webhooks == nil {
		webhooks = make(map[string]Webhook)
	}
	updatePolicies = cfg.UpdatePolicies
	if updatePolicies == nil {
		updatePolicies = make(map[string]UpdatePolicy)
	}
	
	// Phase 4
	registries = cfg.Registries
	if registries == nil {
		registries = make(map[string]Registry)
	}
}

// Save configuration to file
func saveConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	cfg := Config{
		AutoUpdateContainers: autoUpdateContainers,
		TrackedImages:        trackedImages,
		TrackedCharts:        trackedCharts,
		LastCheck:            time.Now(),
		RollbackHistory:      rollbackHistory,
		Templates:            templates,
		MaintenanceMode:      maintenanceMode,
		MaintenancePaused:    maintenancePaused,
		ResourceAlerts:       resourceAlerts,
		HealthChecks:         healthChecks,
		ReportSchedule:       reportSchedule,
		LastReportTime:       lastReportTime,
		AuditLog:             auditLog,
		Webhooks:             webhooks,
		UpdatePolicies:       updatePolicies,
		Registries:           registries,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}

	// Create directory if it doesn't exist
	os.MkdirAll("/data", 0755)

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		log.Printf("Error saving config: %v", err)
	}
}

// Load language translations
func loadLanguage(lang string) error {
	data, err := os.ReadFile(fmt.Sprintf("/app/locale/%s.json", lang))
	if err != nil {
		return err
	}
	
	return json.Unmarshal(data, &translations)
}

// Get translated text with placeholder replacement
func getText(key string, args ...interface{}) string {
	text, ok := translations[key]
	if !ok {
		return key // Return key if translation not found
	}
	
	// Replace placeholders $1, $2, etc.
	for i, arg := range args {
		placeholder := fmt.Sprintf("$%d", i+1)
		text = strings.ReplaceAll(text, placeholder, fmt.Sprint(arg))
	}
	
	return text
}

func getIcon(name string) string {
	name = strings.ToLower(name)
	for key, icon := range containerIcons {
		if strings.Contains(name, key) {
			return icon
		}
	}
	return "📦"
}

func addCloseButton(keyboard tgbotapi.InlineKeyboardMarkup) tgbotapi.InlineKeyboardMarkup {
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	return keyboard
}

func sendMessageWithClose(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func editToLoading(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, "⏳ "+text)
	edit.ParseMode = "Markdown"
	bot.Send(edit)
}

func sendLoading(chatID int64, text string) int {
	msg := tgbotapi.NewMessage(chatID, "⏳ "+text)
	msg.ParseMode = "Markdown"
	sent, _ := bot.Send(msg)
	return sent.MessageID
}

func deleteMsg(chatID int64, messageID int) {
	bot.Send(tgbotapi.NewDeleteMessage(chatID, messageID))
}

func runCmd(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	return string(out), err
}

func runCmdWithTimeout(timeout time.Duration, cmd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(ctx, cmd, args...)
	out, err := command.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("comando excedió timeout de %v", timeout)
	}

	return string(out), err
}

func findComposeFile(workDir string) string {
	possibleFiles := []string{
		workDir + "/compose.yaml",
		workDir + "/compose.yml",
		workDir + "/docker-compose.yaml",
		workDir + "/docker-compose.yml",
	}

	for _, file := range possibleFiles {
		if _, err := os.Stat(file); err == nil {
			return file
		}
	}

	return ""
}

func validateComposeSetup() error {
	out, err := runCmd("docker", "compose", "version")
	if err != nil {
		return fmt.Errorf("docker compose no está disponible: %v", err)
	}
	log.Printf("Docker Compose version: %s", strings.TrimSpace(out))
	return nil
}

func normalizeID(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
}

func getStats() map[string]struct{ CPU, Mem string } {
	ctx := context.Background()
	stats := make(map[string]struct{ CPU, Mem string })
	var mu sync.Mutex

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return stats
	}

	var wg sync.WaitGroup
	for _, c := range containers {
		wg.Add(1)
		go func(cont types.Container) {
			defer wg.Done()
			
			name := strings.TrimPrefix(cont.Names[0], "/")
			statsResp, err := cli.ContainerStats(ctx, cont.ID, false)
			if err != nil {
				return
			}
			defer statsResp.Body.Close()

			var v container.StatsResponse
			if err := json.NewDecoder(statsResp.Body).Decode(&v); err != nil {
				return
			}

			cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
			cpuPercent := 0.0
			if systemDelta > 0 && cpuDelta > 0 {
				cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
			}

			memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024
			memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024

			mu.Lock()
			stats[name] = struct{ CPU, Mem string }{
				fmt.Sprintf("%.2f%%", cpuPercent),
				fmt.Sprintf("%.0fMiB / %.0fMiB", memUsage, memLimit),
			}
			mu.Unlock()
		}(c)
	}

	wg.Wait()
	return stats
}

func recreateWithNewImage(name string) error {
	ctx := context.Background()

	// Inspect container
	inspect, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect failed: %w", err)
	}

	wasRunning := inspect.State.Running
	imageTag := inspect.Config.Image

	// Save rollback entry before updating
	saveRollbackEntry(name, imageTag, inspect.Image)

	// Stop container
	timeout := 10
	if err := cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}

	// Rename old
	oldName := name + "_old"
	cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true})
	if err := cli.ContainerRename(ctx, name, oldName); err != nil {
		cli.ContainerStart(ctx, name, container.StartOptions{})
		return fmt.Errorf("rename failed: %w", err)
	}

	// Create new
	resp, err := cli.ContainerCreate(ctx, inspect.Config, inspect.HostConfig, &network.NetworkingConfig{
		EndpointsConfig: inspect.NetworkSettings.Networks,
	}, nil, name)
	if err != nil {
		cli.ContainerRename(ctx, oldName, name)
		if wasRunning {
			cli.ContainerStart(ctx, name, container.StartOptions{})
		}
		return fmt.Errorf("create failed: %w", err)
	}

	// Start new
	if wasRunning {
		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
			cli.ContainerRename(ctx, oldName, name)
			cli.ContainerStart(ctx, name, container.StartOptions{})
			return fmt.Errorf("start failed: %w", err)
		}

		// Verify running
		for i := 0; i < 5; i++ {
			time.Sleep(time.Second)
			check, _ := cli.ContainerInspect(ctx, name)
			if check.State.Running {
				break
			}
			if i == 4 {
				cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
				cli.ContainerRename(ctx, oldName, name)
				cli.ContainerStart(ctx, name, container.StartOptions{})
				return fmt.Errorf("container exited after start")
			}
		}
	}

	// Remove old
	cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true})
	log.Printf("[recreate] ✅ %s recreated with new image %s", name, imageTag)
	return nil
}

func checkBotVersion(chatID int64) {
	// Check if there's a new version available on GitHub
	// This is a simple implementation - you can enhance it to check GitHub releases API
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🤖 *Botainer v%s*\n\n📢 Mantente al día con las últimas novedades y actualizaciones:", botVersion))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📢 Canal de Novedades", newsChannelURL),
			tgbotapi.NewInlineKeyboardButtonURL("⭐ GitHub", "https://github.com/YonierGomez/botainer"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func handleStart(chatID int64) {
	// Check bot version and show update notification if available
	checkBotVersion(chatID)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Lista", "cmd:list"),
			tgbotapi.NewInlineKeyboardButtonData("📊 PS", "cmd:ps"),
			tgbotapi.NewInlineKeyboardButtonData("🖥️ Stats", "cmd:stats"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📁 Compose", "cmd:compose"),
			tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "cmd:inspect_menu"),
			tgbotapi.NewInlineKeyboardButtonData("⚙️ Exec", "cmd:exec_menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🖼️ Images", "cmd:images"),
			tgbotapi.NewInlineKeyboardButtonData("💾 Volumes", "cmd:volumes"),
			tgbotapi.NewInlineKeyboardButtonData("🌐 Networks", "cmd:networks"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 Buscar updates", "cmd:check_updates"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Prune", "cmd:prune_menu"),
			tgbotapi.NewInlineKeyboardButtonData("🔧 Diagnose", "cmd:diagnose"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "🐳 *Botainer*\nGestiona tus contenedores Docker")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleNetworks(chatID int64) {
	ctx := context.Background()
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	for _, net := range networks {
		containers := []string{}
		for _, ep := range net.Containers {
			containers = append(containers, ep.Name)
		}

		project := net.Labels["com.docker.compose.project"]

		text := fmt.Sprintf("🌐 *%s*\n   ├ Driver: `%s`\n   ├ Scope: `%s`", net.Name, net.Driver, net.Scope)
		if len(containers) > 0 {
			text += fmt.Sprintf("\n   ├ Contenedores: `%s`", strings.Join(containers, ", "))
		}
		if project != "" {
			text += fmt.Sprintf("\n   └ Proyecto: `%s`", project)
		} else {
			text += "\n   └ Sin contenedores"
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_net:"+net.Name),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmnet_confirm:"+net.Name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handleImages(chatID int64) {
	ctx := context.Background()
	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	for _, img := range images {
		tag := "<none>"
		if len(img.RepoTags) > 0 {
			tag = img.RepoTags[0]
		}

		sizeMB := float64(img.Size) / 1024 / 1024
		sizeText := fmt.Sprintf("%.1f MB", sizeMB)
		if sizeMB > 1024 {
			sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
		}

		text := fmt.Sprintf("🖼️ *%s*\n   ├ ID: `%s`\n   └ Tamaño: `%s`", tag, img.ID[:19], sizeText)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_img:"+img.ID),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmi_confirm:"+img.ID),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handleVolumes(chatID int64) {
	ctx := context.Background()
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	for _, vol := range volumes.Volumes {
		// Find containers using this volume
		containers, _ := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("volume", vol.Name)),
		})

		containerNames := []string{}
		for _, c := range containers {
			containerNames = append(containerNames, strings.TrimPrefix(c.Names[0], "/"))
		}

		project := vol.Labels["com.docker.compose.project"]

		var text string
		if len(containerNames) > 0 {
			text = fmt.Sprintf("💾 *%s*\n   ├ Usado por: `%s`", vol.Name, strings.Join(containerNames, ", "))
			if project != "" {
				text += fmt.Sprintf("\n   └ Proyecto: `%s`", project)
			}
		} else if project != "" {
			text = fmt.Sprintf("💾 *%s*\n   └ Proyecto: `%s`", vol.Name, project)
		} else {
			text = fmt.Sprintf("💾 *%s*\n   └ Sin usar", vol.Name)
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_vol:"+vol.Name),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmvol_confirm:"+vol.Name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💾 Backup", "backup:"+vol.Name),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handlePS(chatID int64) {
	loadingID := sendLoading(chatID, "Obteniendo estadísticas...")
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores en ejecución")
		return
	}

	type result struct {
		name, icon, status, image, project, cpu, mem string
	}

	results := make(chan result, len(containers))

	for _, c := range containers {
		go func(c types.Container) {
			name := strings.TrimPrefix(c.Names[0], "/")
			icon := getIcon(name)

			inspect, _ := cli.ContainerInspect(ctx, c.ID)
			project := inspect.Config.Labels["com.docker.compose.project"]

			cpu, mem := "N/A", "N/A"
			statsResp, err := cli.ContainerStats(ctx, c.ID, false)
			if err == nil {
				var v container.StatsResponse
				if json.NewDecoder(statsResp.Body).Decode(&v) == nil {
					cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
					systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
					
					// Use OnlineCPUs if available, otherwise use PercpuUsage length
					numCPU := v.CPUStats.OnlineCPUs
					if numCPU == 0 {
						numCPU = uint32(len(v.CPUStats.CPUUsage.PercpuUsage))
					}
					
					if systemDelta > 0 && numCPU > 0 {
						cpuPercent := (cpuDelta / systemDelta) * float64(numCPU) * 100.0
						cpu = fmt.Sprintf("%.1f%%", cpuPercent)
					} else {
						cpu = "0.0%"
					}
					
					memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 / 1024
					memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 / 1024
					mem = fmt.Sprintf("%.2fGB / %.2fGB", memUsage, memLimit)
				}
				statsResp.Body.Close()
			}

			results <- result{name, icon, c.Status, c.Image, project, cpu, mem}
		}(c)
	}

	for i := 0; i < len(containers); i++ {
		r := <-results

		text := fmt.Sprintf("🟢 %s *%s*\n   ├ Estado: `%s`\n   ├ Imagen: `%s`", r.icon, r.name, r.status, r.image)
		if r.project != "" {
			text += fmt.Sprintf("\n   ├ Proyecto: `%s`", r.project)
		}
		text += fmt.Sprintf("\n   ├ CPU: `%s`\n   └ RAM: `%s`", r.cpu, r.mem)

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+r.name),
				tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+r.name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+r.name),
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+r.name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
	}
}

func handleRunning(chatID int64) {
	loadingID := sendLoading(chatID, "Cargando contenedores...")
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		icon := getIcon(name)
		statusIcon := "🔴"
		if c.State == "running" {
			statusIcon = "🟢"
		} else if c.State == "paused" {
			statusIcon = "🟡"
		}

		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		project := inspect.Config.Labels["com.docker.compose.project"]

		text := fmt.Sprintf("%s %s *%s*\n   ├ Estado: `%s`\n   └ Imagen: `%s`", statusIcon, icon, name, c.Status, c.Image)
		if project != "" {
			text = fmt.Sprintf("%s %s *%s*\n   ├ Estado: `%s`\n   ├ Imagen: `%s`\n   └ Proyecto: `%s`", statusIcon, icon, name, c.Status, c.Image, project)
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		var keyboard tgbotapi.InlineKeyboardMarkup
		if c.State == "running" {
			keyboard = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
					tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+name),
					tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
		} else {
			keyboard = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("▶️ Start", "start:"+name),
					tgbotapi.NewInlineKeyboardButtonData("🗑️ Remove", "remove:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
		}
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handleList(chatID int64) {
	loadingID := sendLoading(chatID, "Listando contenedores...")
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		dot1 := "🔴"
		if containers[i].State == "running" {
			dot1 = "🟢"
		} else if containers[i].State == "paused" {
			dot1 = "🟡"
		}

		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(dot1+" "+getIcon(name1)+" "+name1, "container_menu:"+name1),
		}

		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			dot2 := "🔴"
			if containers[i+1].State == "running" {
				dot2 = "🟢"
			} else if containers[i+1].State == "paused" {
				dot2 = "🟡"
			}
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(dot2+" "+getIcon(name2)+" "+name2, "container_menu:"+name2))
		}
		keyboard = append(keyboard, row)
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🐳 *Contenedores* (%d)", len(containers)))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}
func handleGrid(chatID int64, title, action string, allContainers bool) {
	ctx := context.Background()
	opts := container.ListOptions{}
	if allContainers {
		opts.All = true
	}

	containers, err := cli.ContainerList(ctx, opts)
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		dot1 := "🔴"
		if containers[i].State == "running" {
			dot1 = "🟢"
		} else if containers[i].State == "paused" {
			dot1 = "🟡"
		}

		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(dot1+" "+getIcon(name1)+" "+name1, action+":"+name1),
		}

		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			dot2 := "🔴"
			if containers[i+1].State == "running" {
				dot2 = "🟢"
			} else if containers[i+1].State == "paused" {
				dot2 = "🟡"
			}
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(dot2+" "+getIcon(name2)+" "+name2, action+":"+name2))
		}
		keyboard = append(keyboard, row)
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, title)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}
func handleCallback(query *tgbotapi.CallbackQuery) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in handleCallback: %v", r)
		}
	}()

	if query.Message == nil {
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	chatID := query.Message.Chat.ID
	ctx := context.Background()

	if query.Data == "close" {
		bot.Send(tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	if strings.HasPrefix(query.Data, "newtag_howto:") {
		parts := strings.Split(query.Data, ":")
		if len(parts) >= 4 {
			containerName := parts[1]
			oldTag := parts[2]
			newTag := parts[3]
			
			howto := fmt.Sprintf("📝 *Cómo actualizar %s*\n\n"+
				"Para actualizar de `%s` a `%s`:\n\n"+
				"1️⃣ Edita tu `docker-compose.yml`\n"+
				"2️⃣ Cambia el tag de la imagen:\n"+
				"   `image: %s`\n"+
				"3️⃣ Ejecuta:\n"+
				"   `docker compose up -d %s`\n\n"+
				"💡 _O usa el comando /compose para gestionar tu proyecto_",
				containerName, oldTag, newTag, newTag, containerName)
			
			msg := tgbotapi.NewMessage(chatID, howto)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	if strings.HasPrefix(query.Data, "newtag_update:") {
		// Format: newtag_update:containerName|oldTag|newTag|project
		data := strings.TrimPrefix(query.Data, "newtag_update:")
		parts := strings.Split(data, "|")
		if len(parts) >= 3 {
			containerName := parts[0]
			oldTag := parts[1]
			newTag := parts[2]
			project := ""
			if len(parts) >= 4 {
				project = parts[3]
			}
			
			editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("🔄 Actualizando *%s* a `%s`...", containerName, newTag))
			
			var out string
			
			if project != "" {
				// Compose service - edit compose file and run up
				workDir := getComposeWorkDir(project)
				if workDir == "" {
					out = "❌ No se encontró el directorio del proyecto"
				} else {
					composeFile := findComposeFile(workDir)
					if composeFile == "" {
						out = fmt.Sprintf("❌ No se encontró archivo compose en: `%s`", workDir)
					} else {
						// Use sed to replace the image tag in compose file
						sedCmd := fmt.Sprintf("sed -i 's|image: %s|image: %s|g' %s", oldTag, newTag, composeFile)
						sedOut, sedErr := runCmdWithTimeout(30*time.Second, "sh", "-c", sedCmd)
						
						if sedErr != nil {
							out = fmt.Sprintf("❌ Error al editar compose: %v\n%s", sedErr, sedOut)
						} else {
							// Run docker compose up -d for the service
							upOut, upErr := runCmdWithTimeout(2*time.Minute, "docker", "compose", "-f", composeFile, "up", "-d", "--remove-orphans", containerName)
							if upErr != nil {
								out = fmt.Sprintf("❌ Error al actualizar: %v\n%s", upErr, upOut)
							} else {
								out = fmt.Sprintf("✅ *%s* actualizado a `%s`\n\n_Compose file modificado y servicio actualizado_", containerName, newTag)
							}
						}
					}
				}
			} else {
				// Standalone container - recreate with new image
				inspect, err := cli.ContainerInspect(ctx, containerName)
				if err != nil {
					out = fmt.Sprintf("❌ Error al inspeccionar contenedor: %v", err)
				} else {
					// Pull new image first using Docker API
					pullResp, pullErr := cli.ImagePull(ctx, newTag, image.PullOptions{})
					if pullErr != nil {
						out = fmt.Sprintf("❌ Error al descargar imagen: %v", pullErr)
					} else {
						// Consume the pull response to ensure it completes
						io.Copy(io.Discard, pullResp)
						pullResp.Close()
						
						// Stop and remove old container
						cli.ContainerStop(ctx, containerName, container.StopOptions{})
						cli.ContainerRemove(ctx, containerName, container.RemoveOptions{})
						
						// Create new container with new image
						config := inspect.Config
						config.Image = newTag
						
						// Build network config
						networkConfig := &network.NetworkingConfig{
							EndpointsConfig: inspect.NetworkSettings.Networks,
						}
						
						resp, err := cli.ContainerCreate(ctx, config, inspect.HostConfig, networkConfig, nil, containerName)
						if err != nil {
							out = fmt.Sprintf("❌ Error al crear contenedor: %v", err)
						} else {
							// Start new container
							if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
								out = fmt.Sprintf("❌ Error al iniciar contenedor: %v", err)
							} else {
								out = fmt.Sprintf("✅ *%s* actualizado a `%s`", containerName, newTag)
							}
						}
					}
				}
			}
			
			msg := tgbotapi.NewMessage(chatID, out)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	parts := strings.SplitN(query.Data, ":", 2)
	if len(parts) != 2 {
		log.Printf("Invalid callback data: %s", query.Data)
		return
	}
	action, target := parts[0], parts[1]

	var out string
	var err error

	switch action {
	case "cmd":
		switch target {
		case "ps":
			go handlePS(chatID)
		case "stats":
			go handleStats(chatID)
		case "compose":
			go handleCompose(chatID)
		case "inspect_menu":
			go handleInspectMenu(chatID)
		case "exec_menu":
			go handleExecMenu(chatID)
		case "prune_menu":
			go handlePrune(chatID)
		case "restart":
			go handleGrid(chatID, "🔄 *Reiniciar contenedor*", "restart", false)
		case "logs":
			go handleGrid(chatID, "📊 *Ver logs*", "logs", false)
		case "stop":
			go handleGrid(chatID, "⏸️ *Detener contenedor*", "stop", false)
		case "images":
			go handleImages(chatID)
		case "volumes":
			go handleVolumes(chatID)
		case "networks":
			go handleNetworks(chatID)
		case "check_updates":
			go func() {
				sendMessageWithClose(chatID, "🔍 Buscando actualizaciones de imágenes...")
				runImageUpdateCheckWithFeedback(chatID)
			}()
		case "trackimage":
			go handleTrackImage(chatID)
		case "trackchart":
			go handleTrackChart(chatID)
		case "list":
			go handleList(chatID)
		case "diagnose":
			go handleDiagnose(chatID)
		case "templates":
			go handleTemplates(chatID)
		case "rollback":
			go handleRollback(chatID)
		case "maintenance":
			go handleMaintenance(chatID)
		case "alerts":
			go handleAlerts(chatID)
		case "healthchecks":
			go handleHealthChecks(chatID)
		case "reports":
			go handleReports(chatID)
		case "audit":
			go handleAudit(chatID)
		case "scan":
			go handleScan(chatID)
		case "webhooks":
			go handleWebhooks(chatID)
		case "policies":
			go handlePolicies(chatID)
		case "registries":
			go handleRegistries(chatID)
		case "cleanup":
			go handleCleanup(chatID)
		case "ports":
			go handlePorts(chatID)
		case "inspect_containers":
			go handleList(chatID)
		case "inspect_images":
			images, _ := cli.ImageList(ctx, image.ListOptions{})
			var keyboard [][]tgbotapi.InlineKeyboardButton
			for i := 0; i < len(images); i += 2 {
				tag1 := "<none>"
				if len(images[i].RepoTags) > 0 {
					tag1 = images[i].RepoTags[0]
				}
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData("🖼️ "+tag1, "inspect_img:"+images[i].ID),
				}
				if i+1 < len(images) {
					tag2 := "<none>"
					if len(images[i+1].RepoTags) > 0 {
						tag2 = images[i+1].RepoTags[0]
					}
					row = append(row, tgbotapi.NewInlineKeyboardButtonData("🖼️ "+tag2, "inspect_img:"+images[i+1].ID))
				}
				keyboard = append(keyboard, row)
			}
			msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar imagen*")
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
			bot.Send(msg)
		case "inspect_volumes":
			volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
			var keyboard [][]tgbotapi.InlineKeyboardButton
			for i := 0; i < len(volumes.Volumes); i += 2 {
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData("💾 "+volumes.Volumes[i].Name, "inspect_vol:"+volumes.Volumes[i].Name),
				}
				if i+1 < len(volumes.Volumes) {
					row = append(row, tgbotapi.NewInlineKeyboardButtonData("💾 "+volumes.Volumes[i+1].Name, "inspect_vol:"+volumes.Volumes[i+1].Name))
				}
				keyboard = append(keyboard, row)
			}
			msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar volumen*")
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
			bot.Send(msg)
		case "inspect_networks":
			networks, _ := cli.NetworkList(ctx, network.ListOptions{})
			var keyboard [][]tgbotapi.InlineKeyboardButton
			for i := 0; i < len(networks); i += 2 {
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData("🌐 "+networks[i].Name, "inspect_net:"+networks[i].Name),
				}
				if i+1 < len(networks) {
					row = append(row, tgbotapi.NewInlineKeyboardButtonData("🌐 "+networks[i+1].Name, "inspect_net:"+networks[i+1].Name))
				}
				keyboard = append(keyboard, row)
			}
			keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:inspect_menu"),
			))
			msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar red*")
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
			bot.Send(msg)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "restart":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Reiniciando *%s*...", target))
		timeout := 10
		err = cli.ContainerRestart(ctx, target, container.StopOptions{Timeout: &timeout})
		if err == nil {
			out = fmt.Sprintf("✅ *%s* reiniciado", target)
		}

	case "stop":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Deteniendo *%s*...", target))
		timeout := 10
		err = cli.ContainerStop(ctx, target, container.StopOptions{Timeout: &timeout})
		if err == nil {
			out = fmt.Sprintf("✅ *%s* detenido", target)
		}

	case "start":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Iniciando *%s*...", target))
		err = cli.ContainerStart(ctx, target, container.StartOptions{})
		if err == nil {
			time.Sleep(2 * time.Second)
			inspect, _ := cli.ContainerInspect(ctx, target)
			if inspect.State.Running {
				stats := getStats()
				stat := stats[target]
				icon := getIcon(target)
				out = fmt.Sprintf("✅ %s *%s* iniciado\n\n🟢 Estado: `running`\n📊 CPU: `%s`\n💾 RAM: `%s`", icon, target, stat.CPU, stat.Mem)
			} else {
				logsReader, _ := cli.ContainerLogs(ctx, target, container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Tail:       "20",
				})
				logsBytes, _ := io.ReadAll(logsReader)
				logsReader.Close()
				icon := getIcon(target)
				out = fmt.Sprintf("⚠️ %s *%s* no inició correctamente\n\n🔴 Estado: `%s`\n\n📋 Últimos logs:\n```\n%s\n```", icon, target, inspect.State.Status, string(logsBytes))
			}
		}

	case "remove":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Eliminando *%s*...", target))
		err = cli.ContainerRemove(ctx, target, container.RemoveOptions{})
		if err == nil {
			out = fmt.Sprintf("✅ *%s* eliminado", target)
		} else {
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ No se pudo eliminar *%s*\n\n```\n%s\n```\n\n¿Deseas forzar la eliminación?", target, err.Error()))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("💪 Forzar eliminación", "remove_force:"+target),
					tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
				),
			)
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}

	case "remove_force":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Forzando eliminación de *%s*...", target))
		err = cli.ContainerRemove(ctx, target, container.RemoveOptions{Force: true, RemoveVolumes: true})
		if err == nil {
			out = fmt.Sprintf("✅ *%s* eliminado forzadamente", target)
		}

	case "pause":
		err = cli.ContainerPause(ctx, target)
		if err == nil {
			out = fmt.Sprintf("⏸️ *%s* pausado", target)
		}

	case "unpause":
		err = cli.ContainerUnpause(ctx, target)
		if err == nil {
			out = fmt.Sprintf("▶️ *%s* reanudado", target)
		}

	case "logs":
		logsReader, err := cli.ContainerLogs(ctx, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "30",
		})
		if err == nil {
			logsBytes, _ := io.ReadAll(logsReader)
			logsReader.Close()
			logs := string(logsBytes)

			lines := strings.Split(logs, "\n")
			highlighted := []string{}
			for _, line := range lines {
				lineLower := strings.ToLower(line)
				if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "fatal") {
					highlighted = append(highlighted, "🔴 "+line)
				} else if strings.Contains(lineLower, "warn") {
					highlighted = append(highlighted, "🟡 "+line)
				} else {
					highlighted = append(highlighted, line)
				}
			}
			out = fmt.Sprintf("📊 *Logs de %s*\n```\n%s\n```", target, strings.Join(highlighted, "\n"))

			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔴 Errors", "logs_filter:"+target+":error"),
					tgbotapi.NewInlineKeyboardButtonData("🟡 Warnings", "logs_filter:"+target+":warn"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📊 Más logs", "logs_more:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "logs:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("💾 Descargar .log", "logfile:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
			msg := tgbotapi.NewMessage(chatID, out)
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}

	case "logs_filter":
		parts := strings.SplitN(target, ":", 2)
		if len(parts) == 2 {
			containerName, filter := parts[0], parts[1]
			logsReader, _ := cli.ContainerLogs(ctx, containerName, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Tail:       "100",
			})
			logsBytes, _ := io.ReadAll(logsReader)
			logsReader.Close()
			lines := strings.Split(string(logsBytes), "\n")
			filtered := []string{}
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), filter) {
					filtered = append(filtered, line)
				}
			}
			if len(filtered) > 0 {
				out = fmt.Sprintf("📊 *Logs filtrados (%s) de %s*\n```\n%s\n```", filter, containerName, strings.Join(filtered, "\n"))
			} else {
				out = fmt.Sprintf("No se encontraron logs con '%s'", filter)
			}
		}

	case "logs_more":
		logsReader, _ := cli.ContainerLogs(ctx, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "100",
		})
		logsBytes, _ := io.ReadAll(logsReader)
		logsReader.Close()
		out = fmt.Sprintf("📊 *Logs completos de %s*\n```\n%s\n```", target, string(logsBytes))

	case "logfile":
		logsReader, err := cli.ContainerLogs(ctx, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "1000",
		})
		if err == nil {
			logsBytes, _ := io.ReadAll(logsReader)
			logsReader.Close()
			filename := fmt.Sprintf("/tmp/%s_%d.log", target, time.Now().Unix())
			os.WriteFile(filename, logsBytes, 0644)
			doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filename))
			doc.Caption = fmt.Sprintf("📋 Logs de *%s*", target)
			doc.ParseMode = "Markdown"
			bot.Send(doc)
			os.Remove(filename)
			bot.Request(tgbotapi.NewCallback(query.ID, "✅ Archivo generado"))
			return
		}

	case "inspect":
		inspect, _ := cli.ContainerInspect(ctx, target)
		jsonData, _ := json.MarshalIndent(inspect, "", "  ")
		out = string(jsonData)
		if len(out) > 3800 {
			out = out[:3800] + "\n...\n(truncado)"
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect %s*\n```\n%s\n```", target, out))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "inspect_img":
		imgInspect, _, _ := cli.ImageInspectWithRaw(ctx, target)
		jsonData, _ := json.MarshalIndent(imgInspect, "", "  ")
		out = string(jsonData)
		if len(out) > 3800 {
			out = out[:3800] + "\n...\n(truncado)"
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect imagen*\n```\n%s\n```", out))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "inspect_vol":
		volInspect, _ := cli.VolumeInspect(ctx, target)
		jsonData, _ := json.MarshalIndent(volInspect, "", "  ")
		out = string(jsonData)
		if len(out) > 3800 {
			out = out[:3800] + "\n...\n(truncado)"
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect volumen*\n```\n%s\n```", out))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "inspect_net":
		netInspect, _ := cli.NetworkInspect(ctx, target, network.InspectOptions{})
		jsonData, _ := json.MarshalIndent(netInspect, "", "  ")
		out = string(jsonData)
		if len(out) > 3800 {
			out = out[:3800] + "\n...\n(truncado)"
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect red*\n```\n%s\n```", out))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "rmi":
		_, err = cli.ImageRemove(ctx, target, image.RemoveOptions{})
		if err == nil {
			out = "✅ Imagen eliminada"
		}

	case "rmvol":
		err = cli.VolumeRemove(ctx, target, false)
		if err == nil {
			out = "✅ Volumen eliminado"
		}

	case "rmnet":
		err = cli.NetworkRemove(ctx, target)
		if err == nil {
			out = "✅ Red eliminada"
		}

	case "prune":
		switch target {
		case "images":
			report, err := cli.ImagesPrune(ctx, filters.Args{})
			if err == nil {
				out = fmt.Sprintf("✅ Imágenes no usadas eliminadas\nEspacio liberado: %d bytes", report.SpaceReclaimed)
			}
		case "volumes":
			report, err := cli.VolumesPrune(ctx, filters.Args{})
			if err == nil {
				out = fmt.Sprintf("✅ Volúmenes no usados eliminados\nEspacio liberado: %d bytes", report.SpaceReclaimed)
			}
		case "networks":
			report, err := cli.NetworksPrune(ctx, filters.Args{})
			if err == nil {
				out = fmt.Sprintf("✅ Redes no usadas eliminadas\nRedes eliminadas: %d", len(report.NetworksDeleted))
			}
		case "all":
			imgReport, _ := cli.ImagesPrune(ctx, filters.Args{})
			volReport, _ := cli.VolumesPrune(ctx, filters.Args{})
			netReport, _ := cli.NetworksPrune(ctx, filters.Args{})
			total := imgReport.SpaceReclaimed + volReport.SpaceReclaimed
			out = fmt.Sprintf("✅ Sistema limpiado\nEspacio liberado: %d bytes\nRedes eliminadas: %d", total, len(netReport.NetworksDeleted))
		}

	case "env":
		inspect, _ := cli.ContainerInspect(ctx, target)
		envVars := strings.Join(inspect.Config.Env, "\n")
		if envVars != "" {
			if len(envVars) > 3800 {
				envVars = envVars[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔧 *Variables de entorno de %s*\n```\n%s\n```", target, envVars))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		} else {
			out = fmt.Sprintf("No hay variables de entorno en *%s*", target)
		}

	case "update_recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Recreando *%s* con nueva imagen...", target))
		err = recreateWithNewImage(target)
		if err == nil {
			out = fmt.Sprintf("✅ *%s* recreado con nueva imagen", target)
		}

	case "compose_pullup_service":
		// Format: project:service
		parts := strings.SplitN(target, ":", 2)
		if len(parts) != 2 {
			out = "❌ Formato inválido"
			break
		}
		project, service := parts[0], parts[1]

		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Actualizando *%s*...", service))

		workDir := getComposeWorkDir(project)
		if workDir == "" {
			out = "❌ No se encontró el directorio del proyecto"
			break
		}

		composeFile := findComposeFile(workDir)
		if composeFile == "" {
			out = fmt.Sprintf("❌ No se encontró archivo compose en: `%s`", workDir)
			break
		}

		log.Printf("Updating service %s in project %s with file: %s", service, project, composeFile)

		// Pull only the specific service (timeout 5 minutos)
		pullOut, pullErr := runCmdWithTimeout(5*time.Minute, "docker", "compose", "-f", composeFile, "pull", service)
		if pullErr != nil {
			log.Printf("Compose pull error for %s: %v\nOutput: %s", service, pullErr, pullOut)
			
			isLocalImageError := strings.Contains(pullOut, "pull access denied") || 
				strings.Contains(pullOut, "repository does not exist")
			
			if !isLocalImageError {
				out = fmt.Sprintf("❌ Error al hacer pull:\n```\n%s\n```", pullOut)
				if len(out) > 3800 {
					out = out[:3800] + "\n...\n```"
				}
				break
			}
			log.Printf("Local image detected for %s, continuing with up", service)
		}

		// Up -d only the specific service (timeout 3 minutos)
		upOut, upErr := runCmdWithTimeout(3*time.Minute, "docker", "compose", "-f", composeFile, "up", "-d", "--remove-orphans", service)
		if upErr != nil {
			log.Printf("Compose up error for %s: %v\nOutput: %s", service, upErr, upOut)
			out = fmt.Sprintf("❌ Error al actualizar:\n```\n%s\n```", upOut)
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n```"
			}
			break
		}

		log.Printf("Successfully updated service: %s", service)
		out = fmt.Sprintf("✅ Contenedor *%s* actualizado correctamente", service)

	case "container_menu":
		inspect, _ := cli.ContainerInspect(ctx, target)
		icon := getIcon(target)
		statusIcon := "🔴"
		if inspect.State.Running {
			statusIcon = "🟢"
		} else if inspect.State.Paused {
			statusIcon = "🟡"
		}

		var rows [][]tgbotapi.InlineKeyboardButton
		if inspect.State.Running {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+target),
					tgbotapi.NewInlineKeyboardButtonData("💾 Logfile", "logfile:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+target),
					tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔧 Env", "env:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑️ Remove", "remove_confirm:"+target),
				),
			}
		} else if inspect.State.Paused {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("▶️ Reanudar", "unpause:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+target),
				),
			}
		} else {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("▶️ Start", "start:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🗑️ Remove", "remove_confirm:"+target),
				),
			}
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		))
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s %s *%s*\nEstado: `%s`\n¿Qué deseas hacer?", statusIcon, icon, target, inspect.State.Status))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "remove_confirm", "rmvol_confirm", "rmnet_confirm", "rmi_confirm":
		var confirmText string
		var confirmAction string
		switch action {
		case "remove_confirm":
			confirmText = fmt.Sprintf("⚠️ *¿Eliminar %s?*\nEsta acción no se puede deshacer.", target)
			confirmAction = "remove:" + target
		case "rmvol_confirm":
			confirmText = fmt.Sprintf("⚠️ *¿Eliminar volumen %s?*\nSe perderán todos los datos.", target)
			confirmAction = "rmvol:" + target
		case "rmnet_confirm":
			confirmText = fmt.Sprintf("⚠️ *¿Eliminar red %s?*", target)
			confirmAction = "rmnet:" + target
		case "rmi_confirm":
			confirmText = fmt.Sprintf("⚠️ *¿Eliminar imagen?*\n`%s`", target)
			confirmAction = "rmi:" + target
		}
		msg := tgbotapi.NewMessage(chatID, confirmText)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Sí, eliminar", confirmAction),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "togglefav":
		userID := query.From.ID
		found := false
		newFavs := []string{}
		for _, fav := range favorites[userID] {
			if fav == target {
				found = true
			} else {
				newFavs = append(newFavs, fav)
			}
		}
		if found {
			favorites[userID] = newFavs
			out = fmt.Sprintf("❌ *%s* quitado de favoritos", target)
		} else {
			favorites[userID] = append(favorites[userID], target)
			out = fmt.Sprintf("✅ *%s* agregado a favoritos", target)
		}
		go handleAddFavoriteMenu(chatID, userID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Recreando *%s*...", target))
		if err2 := recreateContainer(target); err2 != nil {
			out = "❌ Error: " + err2.Error()
		} else {
			out = fmt.Sprintf("✅ *%s* recreado con la nueva imagen", target)
		}

	case "backup":
		go func(vol string) {
			loadingID := sendLoading(chatID, fmt.Sprintf("Creando backup del volumen *%s*...", vol))
			filename := fmt.Sprintf("/tmp/backup_%s_%d.tar.gz", vol, time.Now().Unix())
			_, err := runCmd("docker", "run", "--rm",
				"-v", vol+":/data:ro",
				"-v", "/tmp:/backup",
				"alpine", "tar", "czf", "/backup/"+strings.TrimPrefix(filename, "/tmp/"), "-C", "/data", ".")
			deleteMsg(chatID, loadingID)
			if err != nil {
				sendMessageWithClose(chatID, "❌ Error creando backup: "+err.Error())
				return
			}
			doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filename))
			doc.Caption = fmt.Sprintf("💾 Backup del volumen *%s*", vol)
			doc.ParseMode = "Markdown"
			bot.Send(doc)
			os.Remove(filename)
		}(target)
		bot.Request(tgbotapi.NewCallback(query.ID, "⏳ Generando backup..."))
		return

	case "au_add":
		buildAutoUpdateSelector(chatID, query.Message.MessageID, "au_toggle_add")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "au_remove":
		buildAutoUpdateSelector(chatID, query.Message.MessageID, "au_toggle_rem")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "au_toggle_add", "au_toggle_rem":
		autoUpdateContainers[target] = !autoUpdateContainers[target]
		buildAutoUpdateSelector(chatID, query.Message.MessageID, action)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "au_all_add":
		containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
		for _, c := range containers {
			name := strings.TrimPrefix(c.Names[0], "/")
			autoUpdateContainers[name] = true
		}
		buildAutoUpdateSelector(chatID, query.Message.MessageID, "au_toggle_add")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "au_none_add":
		autoUpdateContainers = make(map[string]bool)
		buildAutoUpdateSelector(chatID, query.Message.MessageID, "au_toggle_add")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "au_all_rem":
		autoUpdateContainers = make(map[string]bool)
		saveConfig()
		go handleAutoUpdate(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, "✅ Configuración guardada"))
		return

	case "au_save":
		saveConfig()
		go handleAutoUpdate(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, "✅ Configuración guardada"))
		return

	case "track_add":
		msg := tgbotapi.NewMessage(chatID, "📡 *Agregar imagen para trackear*\n\nEnvía el nombre completo de la imagen:\n\nEjemplos:\n• `nginx:latest`\n• `ghcr.io/user/app:main`\n• `docker.io/library/redis:alpine`\n• `registry.hub.docker.com/postgres:15`")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		userState[query.From.ID] = "waiting_track_image"
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "track_remove":
		if len(trackedImages) == 0 {
			bot.Request(tgbotapi.NewCallback(query.ID, "❌ No hay imágenes trackeadas"))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for img := range trackedImages {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ "+img, "track_del:"+img),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:trackimage"),
		))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📡 *Remover imagen trackeada*\n\nSelecciona la imagen a remover:")
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		bot.Send(edit)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "track_del":
		delete(trackedImages, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallback(query.ID, "✅ Imagen removida"))
		go handleTrackImage(chatID)
		return

	case "track_check":
		go func() {
			bot.Request(tgbotapi.NewCallback(query.ID, "🔍 Verificando..."))
			checkTrackedImages(chatID)
		}()
		return

	case "chart_add":
		msg := tgbotapi.NewMessage(chatID, "📦 *Agregar Helm chart para trackear*\n\nEnvía el nombre del chart o la URL de Artifact Hub:\n\n*Formato 1:* `repo/chart`\n• `bitnami/nginx`\n• `argo/argo-cd`\n\n*Formato 2:* URL completa\n• `https://artifacthub.io/packages/helm/argo/argo-cd`")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		userState[query.From.ID] = "waiting_track_chart"
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "chart_remove":
		if len(trackedCharts) == 0 {
			bot.Request(tgbotapi.NewCallback(query.ID, "❌ No hay charts trackeados"))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for chart := range trackedCharts {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ "+chart, "chart_del:"+chart),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:trackchart"),
		))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "📦 *Remover chart trackeado*\n\nSelecciona el chart a remover:")
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		bot.Send(edit)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "chart_del":
		delete(trackedCharts, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallback(query.ID, "✅ Chart removido"))
		go handleTrackChart(chatID)
		return

	case "chart_check":
		go func() {
			bot.Request(tgbotapi.NewCallback(query.ID, "🔍 Verificando..."))
			checkTrackedCharts(chatID)
		}()
		return

	case "chart_url":
		parts := strings.Split(target, "/")
		if len(parts) == 2 {
			url := fmt.Sprintf("https://artifacthub.io/packages/helm/%s/%s", parts[0], parts[1])
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "🔗 "+url))
		}
		return

	// ── Phase 2: Rollback ──────────────────────────────────────────────────
	case "rollback_container":
		history := rollbackHistory[target]
		if len(history) == 0 {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "No hay historial para este contenedor"))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for i, entry := range history {
			label := fmt.Sprintf("↩️ %s (%s)", entry.Image, entry.Timestamp.Format("02/01 15:04"))
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("rollback_do:%s|%d", target, i)),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
		))
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("↩️ *Rollback de %s*\nSelecciona la versión anterior:", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "rollback_do":
		parts := strings.SplitN(target, "|", 2)
		if len(parts) != 2 {
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
		containerName := parts[0]
		idx, _ := strconv.Atoi(parts[1])
		history := rollbackHistory[containerName]
		if idx < 0 || idx >= len(history) {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "Entrada inválida"))
			return
		}
		entry := history[idx]
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("↩️ Haciendo rollback de *%s* a `%s`...", containerName, entry.Image))
		go func() {
			if err := doRollback(containerName, entry); err != nil {
				sendMessageWithClose(chatID, "❌ Error en rollback: "+err.Error())
			} else {
				sendMessageWithClose(chatID, fmt.Sprintf("✅ *%s* revertido a `%s`", containerName, entry.Image))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "rollback_clear":
		delete(rollbackHistory, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "✅ Historial borrado"))
		return

	// ── Phase 2: Templates ────────────────────────────────────────────────
	case "tpl_save":
		go func() {
			if err := saveTemplate(target); err != nil {
				sendMessageWithClose(chatID, "❌ Error guardando plantilla: "+err.Error())
			} else {
				sendMessageWithClose(chatID, fmt.Sprintf("✅ Plantilla *%s* guardada", target))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, "⏳"))
		return

	case "tpl_save_menu":
		containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
		var rows [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(containers); i += 2 {
			name1 := strings.TrimPrefix(containers[i].Names[0], "/")
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(getIcon(name1)+" "+name1, "tpl_save:"+name1),
			}
			if i+1 < len(containers) {
				name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
				row = append(row, tgbotapi.NewInlineKeyboardButtonData(getIcon(name2)+" "+name2, "tpl_save:"+name2))
			}
			rows = append(rows, row)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:templates"),
		))
		msg := tgbotapi.NewMessage(chatID, "💾 *Guardar como plantilla*\nSelecciona el contenedor:")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "tpl_deploy":
		tpl, ok := templates[target]
		if !ok {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "Plantilla no encontrada"))
			return
		}
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("🚀 Desplegando *%s*...", tpl.Name))
		go func() {
			if err := deployTemplate(tpl); err != nil {
				sendMessageWithClose(chatID, "❌ Error desplegando: "+err.Error())
			} else {
				sendMessageWithClose(chatID, fmt.Sprintf("✅ Contenedor *%s* desplegado desde plantilla", tpl.Name))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "tpl_delete":
		delete(templates, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "✅ Plantilla eliminada"))
		go handleTemplates(chatID)
		return

	case "tpl_info":
		tpl, ok := templates[target]
		if !ok {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, "Plantilla no encontrada"))
			return
		}
		text := fmt.Sprintf("📋 *Plantilla: %s*\n\n🖼️ Imagen: `%s`\n", tpl.Name, tpl.Image)
		if len(tpl.Ports) > 0 {
			text += "🔌 Puertos:\n"
			for h, c := range tpl.Ports {
				text += fmt.Sprintf("  • `%s:%s`\n", h, c)
			}
		}
		if len(tpl.Volumes) > 0 {
			text += "💾 Volúmenes:\n"
			for _, v := range tpl.Volumes {
				text += fmt.Sprintf("  • `%s`\n", v)
			}
		}
		if len(tpl.Env) > 0 {
			text += fmt.Sprintf("🔧 Env vars: %d\n", len(tpl.Env))
		}
		text += fmt.Sprintf("📅 Creada: %s", tpl.CreatedAt.Format("02/01/2006 15:04"))
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🚀 Desplegar", "tpl_deploy:"+target),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Eliminar", "tpl_delete:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:templates"),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	// ── Phase 2: Maintenance mode ─────────────────────────────────────────
	case "maintenance_on":
		editToLoading(chatID, query.Message.MessageID, "🔧 Activando modo mantenimiento...")
		go func() {
			count, err := activateMaintenance()
			if err != nil {
				sendMessageWithClose(chatID, "❌ Error: "+err.Error())
			} else {
				sendMessageWithClose(chatID, fmt.Sprintf("🔧 *Modo mantenimiento activado*\n%d contenedores pausados", count))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "maintenance_off":
		editToLoading(chatID, query.Message.MessageID, "✅ Desactivando modo mantenimiento...")
		go func() {
			count, err := deactivateMaintenance()
			if err != nil {
				sendMessageWithClose(chatID, "❌ Error: "+err.Error())
			} else {
				sendMessageWithClose(chatID, fmt.Sprintf("✅ *Modo mantenimiento desactivado*\n%d contenedores reanudados", count))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "maintenance_status":
		go handleMaintenance(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	
	// Phase 1 callbacks
	case "report_daily":
		reportSchedule = "daily"
		saveConfig()
		sendMessageWithClose(chatID, "✅ Reportes configurados: Diario")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_weekly":
		reportSchedule = "weekly"
		saveConfig()
		sendMessageWithClose(chatID, "✅ Reportes configurados: Semanal")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_disabled":
		reportSchedule = "disabled"
		saveConfig()
		sendMessageWithClose(chatID, "✅ Reportes desactivados")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_now":
		// Trigger immediate report
		lastReportTime = time.Time{}
		sendMessageWithClose(chatID, "📊 Generando reporte...")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	
	// Phase 3 callbacks
	case "audit_export":
		data, _ := json.MarshalIndent(auditLog, "", "  ")
		doc := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: "audit.json", Bytes: data})
		bot.Send(doc)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "audit_clear":
		auditLog = []AuditEntry{}
		saveConfig()
		sendMessageWithClose(chatID, "✅ Registro de auditoría limpiado")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	
	// Phase 4 callbacks
	case "cleanup_all":
		editToLoading(chatID, query.Message.MessageID, "🧹 Limpiando imágenes huérfanas...")
		go func() {
			ctx := context.Background()
			report, err := cli.ImagesPrune(ctx, filters.Args{})
			if err != nil {
				sendMessageWithClose(chatID, "❌ Error: "+err.Error())
				return
			}
			sizeMB := float64(report.SpaceReclaimed) / 1024 / 1024
			sizeText := fmt.Sprintf("%.1f MB", sizeMB)
			if sizeMB > 1024 {
				sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
			}
			sendMessageWithClose(chatID, fmt.Sprintf("✅ Limpieza completada\n\nEspacio liberado: %s", sizeText))
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	// Webhook callbacks
	if query.Data == "webhook_add" {
		userState[query.From.ID] = "webhook_name"
		sendMessageWithClose(chatID, "📝 *Nuevo Webhook*\n\nEscribe un nombre para el webhook:")
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	if query.Data == "webhook_manual" {
		text := "📖 *Configuración Manual*\n\n"
		text += "Edita `/data/config.json` y agrega:\n\n"
		text += "```json\n"
		text += `"webhooks": {
  "discord": {
    "url": "https://discord.com/api/webhooks/...",
    "events": ["container.start", "container.stop"],
    "headers": {},
    "enabled": true
  }
}` + "\n```"
		sendMessageWithClose(chatID, text)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	// Webhook event selection
	if strings.HasPrefix(query.Data, "wh_event:") {
		event := strings.TrimPrefix(query.Data, "wh_event:")
		userID := query.From.ID
		
		if createData[userID] == nil {
			createData[userID] = make(map[string]string)
		}
		
		// Get current events
		eventsStr := createData[userID]["webhook_events"]
		events := []string{}
		if eventsStr != "" {
			events = strings.Split(eventsStr, ",")
		}
		
		// Toggle event
		found := false
		newEvents := []string{}
		for _, e := range events {
			if e == event {
				found = true
			} else {
				newEvents = append(newEvents, e)
			}
		}
		
		if !found {
			if event == "all" {
				newEvents = []string{"all"}
			} else {
				// Remove "all" if adding specific event
				filtered := []string{}
				for _, e := range newEvents {
					if e != "all" {
						filtered = append(filtered, e)
					}
				}
				filtered = append(filtered, event)
				newEvents = filtered
			}
		}
		
		createData[userID]["webhook_events"] = strings.Join(newEvents, ",")
		
		// Update message
		text := fmt.Sprintf("📋 Eventos seleccionados: %s\n\nPuedes elegir más o guardar.", strings.Join(newEvents, ", "))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ReplyMarkup = query.Message.ReplyMarkup
		bot.Send(edit)
		
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	// Save webhook
	if query.Data == "wh_save" {
		userID := query.From.ID
		data := createData[userID]
		
		if data == nil || data["webhook_name"] == "" || data["webhook_url"] == "" {
			sendMessageWithClose(chatID, "❌ Error: Datos incompletos")
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
		
		eventsStr := data["webhook_events"]
		if eventsStr == "" {
			eventsStr = "all"
		}
		
		webhooks[data["webhook_name"]] = Webhook{
			URL:     data["webhook_url"],
			Events:  strings.Split(eventsStr, ","),
			Headers: make(map[string]string),
			Enabled: true,
		}
		
		saveConfig()
		delete(userState, userID)
		delete(createData, userID)
		
		sendMessageWithClose(chatID, fmt.Sprintf("✅ Webhook *%s* creado correctamente", data["webhook_name"]))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	// Scan callback
	if strings.HasPrefix(query.Data, "scan:") {
		containerName := strings.TrimPrefix(query.Data, "scan:")
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("🔍 Escaneando *%s*...", containerName))
		
		go func() {
			ctx := context.Background()
			inspect, err := cli.ContainerInspect(ctx, containerName)
			if err != nil {
				sendMessageWithClose(chatID, "❌ Error: "+err.Error())
				return
			}
			
			imageName := inspect.Config.Image
			result, err := scanImage(imageName)
			if err != nil {
				sendMessageWithClose(chatID, fmt.Sprintf("❌ Error al escanear:\n%v\n\n💡 Instala Trivy: `docker run aquasec/trivy`", err))
				return
			}
			
			sendMessageWithClose(chatID, result)
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	if err != nil {
		out = "❌ Error: " + err.Error()
		log.Printf("Error in callback %s: %v", action, err)
	}

	loadingActions := map[string]bool{
		"restart": true, "stop": true, "start": true,
	}
	if loadingActions[action] {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, out)
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close")},
		}}
		bot.Send(edit)
	} else if out != "" {
		msg := tgbotapi.NewMessage(chatID, out)
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
	bot.Request(tgbotapi.NewCallback(query.ID, ""))
}
func monitorEvents() {
	ctx := context.Background()

	for {
		eventsChan, errChan := cli.Events(ctx, events.ListOptions{})

		for {
			select {
			case event := <-eventsChan:
				if notifyChatID == 0 {
					continue
				}

				if event.Type != "container" {
					continue
				}

				name := event.Actor.Attributes["name"]
				image := event.Actor.Attributes["image"]
				exitCode := event.Actor.Attributes["exitCode"]

				if name == "" {
					continue
				}

				icon := getIcon(name)
				now := time.Now().Format("02/01 15:04:05")

				type notification struct {
					text    string
					buttons [][]tgbotapi.InlineKeyboardButton
				}
				var n *notification

				switch event.Action {
				case "start":
					n = &notification{
						text: fmt.Sprintf("🟢 *Contenedor iniciado*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
								tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+name),
							),
						},
					}
				case "stop":
					n = &notification{
						text: fmt.Sprintf("🔴 *Contenedor detenido*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("▶️ Start", "start:"+name),
								tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
							),
						},
					}
				case "die":
					exitInfo := ""
					if exitCode != "" && exitCode != "0" {
						exitInfo = fmt.Sprintf("\n💀 Exit code: `%s`", exitCode)
					}

					logsReader, err := cli.ContainerLogs(ctx, name, container.LogsOptions{
						ShowStdout: true,
						ShowStderr: true,
						Tail:       "5",
					})
					lastLogs := ""
					if err == nil {
						logBytes, _ := io.ReadAll(logsReader)
						logsReader.Close()
						lastLogs = string(logBytes)
						if len(lastLogs) > 500 {
							lastLogs = lastLogs[len(lastLogs)-500:]
						}
					}

					logsSection := ""
					if lastLogs != "" {
						logsSection = fmt.Sprintf("\n\n📋 *Últimos logs:*\n```\n%s\n```", lastLogs)
					}

					n = &notification{
						text: fmt.Sprintf("💥 *Contenedor caído*\n%s *%s*\n📦 `%s`%s\n🕐 %s%s", icon, name, image, exitInfo, now, logsSection),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+name),
								tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
							),
						},
					}
				case "restart":
					n = &notification{
						text: fmt.Sprintf("🔄 *Contenedor reiniciado*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
								tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+name),
							),
						},
					}
				case "destroy":
					n = &notification{
						text: fmt.Sprintf("🗑️ *Contenedor eliminado*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
					}
				case "pause":
					n = &notification{
						text: fmt.Sprintf("⏸️ *Contenedor pausado*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("▶️ Reanudar", "unpause:"+name),
							),
						},
					}
				case "unpause":
					n = &notification{
						text: fmt.Sprintf("▶️ *Contenedor reanudado*\n%s *%s*\n📦 `%s`\n🕐 %s", icon, name, image, now),
					}
				}

				if n != nil {
					m := tgbotapi.NewMessage(notifyChatID, n.text)
					m.ParseMode = "Markdown"
					if len(n.buttons) > 0 {
						m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(n.buttons...)
					}
					bot.Send(m)
				}

			case err := <-errChan:
				if err != nil {
					log.Println("Error monitoring events:", err)
					time.Sleep(5 * time.Second)
				}
				goto reconnect
			}
		}
	reconnect:
		time.Sleep(5 * time.Second)
	}
}
func monitorResourceAlerts() {
	alertedContainers := make(map[string]time.Time)
	pendingAlerts := make(map[string]bool)

	for {
		time.Sleep(5 * time.Minute)

		if notifyChatID == 0 {
			continue
		}

		first := getStats()

		candidates := make(map[string]bool)
		for name, vals := range first {
			var cpu float64
			fmt.Sscanf(strings.TrimSuffix(vals.CPU, "%"), "%f", &cpu)

			// Parse RAM: "234MiB / 15957MiB" -> calculate percentage
			var memUsed, memTotal float64
			memParts := strings.Split(vals.Mem, "/")
			if len(memParts) == 2 {
				fmt.Sscanf(strings.TrimSpace(memParts[0]), "%fMiB", &memUsed)
				fmt.Sscanf(strings.TrimSpace(memParts[1]), "%fMiB", &memTotal)
			}
			memPercent := 0.0
			if memTotal > 0 {
				memPercent = (memUsed / memTotal) * 100
			}

			if cpu > 90 || memPercent > 90 {
				candidates[name] = true
			}
		}

		toAlert := make(map[string]struct{ CPU, Mem string })
		for name := range candidates {
			if pendingAlerts[name] {
				toAlert[name] = first[name]
			}
		}

		pendingAlerts = candidates

		for name, vals := range toAlert {
			if lastAlert, exists := alertedContainers[name]; exists {
				if time.Since(lastAlert) < 30*time.Minute {
					continue
				}
			}

			icon := getIcon(name)
			msg := fmt.Sprintf("⚠️ *Alerta de recursos*\n\n%s *%s*\n🔥 CPU: %s | 💾 RAM: %s", icon, name, vals.CPU, vals.Mem)
			m := tgbotapi.NewMessage(notifyChatID, msg)
			m.ParseMode = "Markdown"
			m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+name),
					tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+name),
				),
			)
			bot.Send(m)
			alertedContainers[name] = time.Now()
		}
	}
}

func scheduledReports() {
	ctx := context.Background()
	weeklyCount := 0

	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		time.Sleep(time.Until(next))

		if notifyChatID == 0 {
			continue
		}

		containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
		runningContainers, _ := cli.ContainerList(ctx, container.ListOptions{})
		images, _ := cli.ImageList(ctx, image.ListOptions{})

		stoppedContainers, _ := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("status", "exited")),
		})

		status := "✅ Todo bien"
		if len(stoppedContainers) > 0 {
			status = "⚠️ Requiere atención"
		}

		report := fmt.Sprintf("📊 *Reporte Diario - %s*\n\n%s\n\n🐳 *Resumen:*\n  • Contenedores: %d (%d corriendo)\n  • Imágenes: %d\n  • Detenidos: %d",
			now.Format("02/01/2006"), status, len(containers), len(runningContainers), len(images), len(stoppedContainers))

		m := tgbotapi.NewMessage(notifyChatID, report)
		m.ParseMode = "Markdown"
		bot.Send(m)

		weeklyCount++
		if weeklyCount >= 7 {
			weeklyCount = 0
			volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
			networks, _ := cli.NetworkList(ctx, network.ListOptions{})

			weekly := fmt.Sprintf("📅 *Reporte Semanal - %s*\n\n%s\n\n🐳 *Docker:*\n  • Contenedores: %d (%d corriendo)\n  • Imágenes: %d\n  • Volúmenes: %d\n  • Redes: %d",
				now.Format("02/01/2006"), status, len(containers), len(runningContainers), len(images), len(volumes.Volumes), len(networks))
			wm := tgbotapi.NewMessage(notifyChatID, weekly)
			wm.ParseMode = "Markdown"
			bot.Send(wm)
		}
	}
}
func checkUpdates() {
	time.Sleep(5 * time.Minute)
	for {
		if enableAutoCheck && notifyChatID != 0 {
			runImageUpdateCheck()
			if len(trackedImages) > 0 {
				checkTrackedImages(notifyChatID)
			}
			if len(trackedCharts) > 0 {
				checkTrackedCharts(notifyChatID)
			}
		}
		time.Sleep(checkUpdatesInterval)
	}
}

func runImageUpdateCheck() int {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return 0
	}

	type containerInfo struct {
		name    string
		project string
	}
	imageMap := make(map[string][]containerInfo)

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		project := inspect.Config.Labels["com.docker.compose.project"]
		imageTag := inspect.Config.Image // Use the tag, not the digest
		imageMap[imageTag] = append(imageMap[imageTag], containerInfo{name, project})
	}

	found := 0
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent checks
	var wg sync.WaitGroup
	
	for imageTag, containers := range imageMap {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire
		
		go func(imgTag string, ctrs []containerInfo) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release
			
			inspect, _ := cli.ContainerInspect(ctx, ctrs[0].name)
			localID := inspect.Image

			reader, err := cli.ImagePull(ctx, imgTag, image.PullOptions{})
			if err == nil {
				io.Copy(io.Discard, reader)
				reader.Close()
			}

			imgInspect, _, _ := cli.ImageInspectWithRaw(ctx, imgTag)
			newID := imgInspect.ID

			// Check for digest-based update (existing logic)
			if localID == "" || newID == "" || localID == newID {
				// No digest change, but check if a newer tag exists (e.g., 3.18 → 3.20)
				// Only for semver tags (skip latest, alpine, etc.)
				if localID != "" && newID != "" && localID == newID {
					// Quick check: only process if tag looks like semver
					parts := strings.Split(imgTag, ":")
					if len(parts) == 2 {
						tag := parts[1]
						// Skip floating tags
						if !skipTags[tag] {
							// Check if tag starts with a number (likely semver)
							if len(tag) > 0 && tag[0] >= '0' && tag[0] <= '9' {
								
								// Use a timeout for tag checking
								done := make(chan bool, 1)
								go func() {
									newerTag, err := findNewerTag(imgTag)
									if err == nil && newerTag != "" {
										log.Printf("Found newer tag: %s → %s", imgTag, newerTag)
										
										icon := getIcon(ctrs[0].name)
										names := make([]string, 0, len(ctrs))
										for _, c := range ctrs {
											names = append(names, c.name)
										}
										
										msgText := fmt.Sprintf("🆕 %s *Nueva versión disponible*\n\n"+
											"📦 *Contenedor(es):* `%s`\n\n"+
											"🔴 *Actual:* `%s`\n"+
											"🟢 *Nueva:* `%s`",
											icon, strings.Join(names, "`, `"), imgTag, newerTag)
										
										// Add action buttons for each container
										var rows [][]tgbotapi.InlineKeyboardButton
										
										for _, c := range ctrs {
											rows = append(rows, tgbotapi.NewInlineKeyboardRow(
												tgbotapi.NewInlineKeyboardButtonData("🔄 Actualizar: "+c.name, "newtag_update:"+c.name+"|"+imgTag+"|"+newerTag+"|"+c.project),
											))
										}
										
										rows = append(rows, tgbotapi.NewInlineKeyboardRow(
											tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
										))
										
										m := tgbotapi.NewMessage(notifyChatID, msgText)
										m.ParseMode = "Markdown"
										m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
										bot.Send(m)
									}
									done <- true
								}()
								
								// Wait max 10 seconds for tag check
								select {
								case <-done:
									// Completed
								case <-time.After(10 * time.Second):
									log.Printf("Timeout checking newer tag for %s", imgTag)
								}
							}
						}
					}
				}
				return
			}
			
			// Digest changed - send update notification
			found++

		oldVer := localID
		newVer := newID
		if len(oldVer) > 19 {
			oldVer = oldVer[len(oldVer)-19:]
		}
		if len(newVer) > 19 {
			newVer = newVer[len(newVer)-19:]
		}

		// Get image size
		sizeMB := float64(imgInspect.Size) / 1024 / 1024
		sizeText := fmt.Sprintf("%.1f MB", sizeMB)
		if sizeMB > 1024 {
			sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
		}

		projectSet := make(map[string]bool)
		for _, c := range containers {
			if c.project != "" {
				projectSet[c.project] = true
			}
		}

		icon := getIcon(containers[0].name)
		names := make([]string, 0, len(containers))
		for _, c := range containers {
			names = append(names, c.name)
		}

		autoUpdated := []string{}
		autoErrors := []string{}
		for _, c := range containers {
			if !autoUpdateContainers[c.name] {
				continue
			}
			if recErr := recreateContainer(c.name); recErr == nil {
				autoUpdated = append(autoUpdated, c.name)
			} else {
				autoErrors = append(autoErrors, c.name+": "+recErr.Error())
			}
		}

		var msgText string
		if len(autoUpdated) > 0 {
			msgText = fmt.Sprintf("🔁 %s *Auto-Update aplicado*\nImagen: `%s`\nTamaño: `%s`\nContenedor(es): `%s`\n\n📦 Versión anterior: `...%s`\n✅ Versión nueva: `...%s`\n\n🚀 Actualizado: `%s`",
				icon, imageTag, sizeText, strings.Join(names, "`, `"), oldVer, newVer, strings.Join(autoUpdated, "`, `"))
			if len(autoErrors) > 0 {
				msgText += "\n⚠️ Errores: " + strings.Join(autoErrors, "; ")
			}
		} else {
			msgText = fmt.Sprintf("🆕 %s *Nueva versión disponible*\nImagen: `%s`\nTamaño: `%s`\nContenedor(es): `%s`\n\n📦 Versión anterior: `...%s`\n✅ Versión nueva: `...%s`",
				icon, imageTag, sizeText, strings.Join(names, "`, `"), oldVer, newVer)
		}

		if len(projectSet) > 0 {
			projects := make([]string, 0, len(projectSet))
			for p := range projectSet {
				projects = append(projects, p)
			}
			msgText += fmt.Sprintf("\nProyecto(s): `%s`", strings.Join(projects, "`, `"))
		}

		m := tgbotapi.NewMessage(notifyChatID, msgText)
		m.ParseMode = "Markdown"

		var rows [][]tgbotapi.InlineKeyboardButton

		// Create buttons for each container with updates (service-specific only)
		for _, c := range containers {
			alreadyDone := false
			for _, au := range autoUpdated {
				if au == c.name {
					alreadyDone = true
					break
				}
			}
			if alreadyDone {
				continue
			}

			if c.project != "" {
				// Compose project: update only the specific service
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Pull & Up: "+c.name, "compose_pullup_service:"+c.project+":"+c.name),
				))
			} else {
				// Standalone container: recreate
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Recrear: "+c.name, "update_recreate:"+c.name),
				))
			}
		}


		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		))
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(m)
		}(imageTag, containers)
	}
	
	wg.Wait() // Wait for all goroutines to finish
	return found
}

func runImageUpdateCheckWithFeedback(chatID int64) {
	// Send initial status message
	totalContainers := 0
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	totalContainers = len(containers)
	
	statusMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("🔄 Verificando %d contenedores...", totalContainers)))
	
	found := runImageUpdateCheck()
	
	// Delete status message
	if statusMsg.MessageID != 0 {
		bot.Request(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
	}
	
	if found == 0 {
		sendMessageWithClose(chatID, "✅ No hay actualizaciones de digest\n\n_Verificando tags más recientes..._")
	}
}
func handleAutoUpdate(chatID int64) {
	enabled := []string{}
	for name := range autoUpdateContainers {
		if autoUpdateContainers[name] {
			enabled = append(enabled, name)
		}
	}

	text := "🔁 *Auto-Update de contenedores*\n\nActualización automática: cuando se detecte una nueva versión, el contenedor se actualizará y recibirás una notificación.\n\n"
	if len(enabled) == 0 {
		text += "📋 Sin contenedores configurados"
	} else {
		text += "✅ Activos: `" + strings.Join(enabled, "`, `") + "`"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Agregar contenedores", "au_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➖ Remover contenedores", "au_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func buildAutoUpdateSelector(chatID int64, messageID int, mode string) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{}
		for j := i; j < i+2 && j < len(containers); j++ {
			name := strings.TrimPrefix(containers[j].Names[0], "/")
			var label string
			if mode == "au_toggle_add" {
				if autoUpdateContainers[name] {
					label = "✅ " + name
				} else {
					label = "⬜ " + name
				}
			} else {
				if autoUpdateContainers[name] {
					label = "🗑️ " + name
				} else {
					label = "— " + name
				}
			}
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, mode+":"+name))
		}
		rows = append(rows, row)
	}

	if mode == "au_toggle_add" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Todos", "au_all_add:_"),
			tgbotapi.NewInlineKeyboardButtonData("⬜ Ninguno", "au_none_add:_"),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Remover todos", "au_all_rem:_"),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("💾 Guardar", "au_save:"+mode),
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	text := "🔁 *Auto-Update — Selecciona contenedores*\n"
	if mode == "au_toggle_add" {
		text += "Toca para activar/desactivar auto-update:"
	} else {
		text += "Toca para marcar los que deseas remover:"
	}

	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
	bot.Send(edit)
}

func handleTrackImage(chatID int64) {
	tracked := []string{}
	for img := range trackedImages {
		tracked = append(tracked, img)
	}

	text := "📡 *Seguimiento de imágenes remotas*\n\nMonitorea actualizaciones de imágenes que no están en contenedores locales.\n\n"
	if len(tracked) == 0 {
		text += "📋 Sin imágenes trackeadas"
	} else {
		text += "✅ Trackeadas:\n"
		for _, img := range tracked {
			digest := trackedImages[img]
			shortDigest := digest
			if len(shortDigest) > 19 {
				shortDigest = "..." + shortDigest[len(shortDigest)-16:]
			}
			text += fmt.Sprintf("• `%s` → `%s`\n", img, shortDigest)
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Agregar imagen", "track_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➖ Remover imagen", "track_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Verificar ahora", "track_check:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func addTrackedImage(chatID int64, imageTag string) {
	imageTag = strings.TrimSpace(imageTag)
	if imageTag == "" {
		sendMessageWithClose(chatID, "❌ Nombre de imagen vacío")
		return
	}

	ctx := context.Background()
	loadingID := sendLoading(chatID, fmt.Sprintf("📡 Verificando imagen `%s`...", imageTag))
	
	reader, err := cli.ImagePull(ctx, imageTag, image.PullOptions{})
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, fmt.Sprintf("❌ Error al verificar imagen:\n```\n%s\n```", err.Error()))
		return
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	imgInspect, _, err := cli.ImageInspectWithRaw(ctx, imageTag)
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, fmt.Sprintf("❌ Error al inspeccionar imagen:\n```\n%s\n```", err.Error()))
		return
	}

	trackedImages[imageTag] = imgInspect.ID
	saveConfig()
	
	deleteMsg(chatID, loadingID)
	sendMessageWithClose(chatID, fmt.Sprintf("✅ Imagen agregada al seguimiento:\n`%s`\n\nDigest: `%s`", imageTag, imgInspect.ID[:19]))
	go handleTrackImage(chatID)
}

func checkTrackedImages(chatID int64) {
	if len(trackedImages) == 0 {
		sendMessageWithClose(chatID, "📋 No hay imágenes trackeadas")
		return
	}

	ctx := context.Background()
	found := 0

	for imageTag, oldID := range trackedImages {
		reader, err := cli.ImagePull(ctx, imageTag, image.PullOptions{})
		if err != nil {
			continue
		}
		io.Copy(io.Discard, reader)
		reader.Close()

		imgInspect, _, err := cli.ImageInspectWithRaw(ctx, imageTag)
		if err != nil || imgInspect.ID == oldID {
			continue
		}

		found++
		trackedImages[imageTag] = imgInspect.ID
		saveConfig()

		oldVer := oldID
		newVer := imgInspect.ID
		if len(oldVer) > 19 {
			oldVer = oldVer[len(oldVer)-19:]
		}
		if len(newVer) > 19 {
			newVer = newVer[len(newVer)-19:]
		}

		sizeMB := float64(imgInspect.Size) / 1024 / 1024
		sizeText := fmt.Sprintf("%.1f MB", sizeMB)
		if sizeMB > 1024 {
			sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
		}

		msgText := fmt.Sprintf("🆕 *Nueva versión disponible*\nImagen trackeada: `%s`\nTamaño: `%s`\n\n📦 Versión anterior: `...%s`\n✅ Versión nueva: `...%s`",
			imageTag, sizeText, oldVer, newVer)

		m := tgbotapi.NewMessage(chatID, msgText)
		m.ParseMode = "Markdown"
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(m)
	}

	if found == 0 {
		sendMessageWithClose(chatID, "✅ Todas las imágenes trackeadas están actualizadas")
	}
}

func handleTrackChart(chatID int64) {
	tracked := []string{}
	for chart := range trackedCharts {
		tracked = append(tracked, chart)
	}

	text := "📦 *Seguimiento de Helm charts*\n\nMonitorea actualizaciones de charts desde Artifact Hub.\n\n"
	if len(tracked) == 0 {
		text += "📋 Sin charts trackeados"
	} else {
		text += "✅ Trackeados:\n"
		for _, chart := range tracked {
			info := trackedCharts[chart]
			text += fmt.Sprintf("• `%s`\n  Chart: `%s` | App: `%s` | Repo: `%s`\n", chart, info.Version, info.AppVersion, info.Repo)
			if len(info.Images) > 0 {
				text += "  🐳 Imágenes:\n"
				for _, img := range info.Images {
					text += fmt.Sprintf("    • `%s`\n", img)
				}
			}
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Agregar chart", "chart_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➖ Remover chart", "chart_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Verificar ahora", "chart_check:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func addTrackedChart(chatID int64, chartName string) {
	chartName = strings.TrimSpace(chartName)
	if chartName == "" {
		sendMessageWithClose(chatID, "❌ Nombre de chart vacío")
		return
	}

	// Extract repo/chart from URL if provided
	if strings.Contains(chartName, "artifacthub.io/packages/helm/") {
		parts := strings.Split(chartName, "/packages/helm/")
		if len(parts) == 2 {
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) >= 2 {
				chartName = pathParts[0] + "/" + pathParts[1]
			}
		}
	}

	loadingID := sendLoading(chatID, fmt.Sprintf("📦 Verificando chart `%s` en Artifact Hub...", chartName))
	
	pkg, err := fetchArtifactHubPackage(chartName)
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, fmt.Sprintf("❌ Error al verificar chart:\n```\n%s\n```\n\nFormato: `repo/chart` (ej: `bitnami/nginx`)", err.Error()))
		return
	}

	images := []string{}
	for _, img := range pkg.ContainersImages {
		if img.Image != "" {
			images = append(images, img.Image)
		}
	}

	trackedCharts[chartName] = ChartInfo{
		Version:    pkg.Version,
		AppVersion: pkg.AppVersion,
		Repo:       pkg.Repository.Name,
		Images:     images,
	}
	saveConfig()
	
	deleteMsg(chatID, loadingID)
	appVer := ""
	if pkg.AppVersion != "" {
		appVer = fmt.Sprintf("\nApp version: `%s`", pkg.AppVersion)
	}
	sendMessageWithClose(chatID, fmt.Sprintf("✅ Chart agregado al seguimiento:\n`%s`\n\nChart version: `%s`%s\nRepo: `%s`", chartName, pkg.Version, appVer, pkg.Repository.Name))
	go handleTrackChart(chatID)
}

func fetchArtifactHubPackage(chartName string) (*ArtifactHubPackage, error) {
	parts := strings.Split(chartName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("formato inválido, usa: repo/chart")
	}

	url := fmt.Sprintf("https://artifacthub.io/api/v1/packages/helm/%s/%s", parts[0], parts[1])
	resp, err := exec.Command("wget", "-qO-", url).Output()
	if err != nil {
		return nil, fmt.Errorf("chart no encontrado")
	}

	var pkg ArtifactHubPackage
	if err := json.Unmarshal(resp, &pkg); err != nil {
		return nil, fmt.Errorf("error al parsear respuesta")
	}

	if pkg.Version == "" {
		return nil, fmt.Errorf("chart no encontrado o sin versión")
	}

	return &pkg, nil
}

func checkTrackedCharts(chatID int64) {
	if len(trackedCharts) == 0 {
		return
	}

	found := 0
	for chartName, oldInfo := range trackedCharts {
		pkg, err := fetchArtifactHubPackage(chartName)
		if err != nil || pkg.Version == oldInfo.Version {
			continue
		}

		found++
		images := []string{}
		for _, img := range pkg.ContainersImages {
			if img.Image != "" {
				images = append(images, img.Image)
			}
		}

		trackedCharts[chartName] = ChartInfo{
			Version:    pkg.Version,
			AppVersion: pkg.AppVersion,
			Repo:       pkg.Repository.Name,
			Images:     images,
		}
		saveConfig()

		appVerText := ""
		if pkg.AppVersion != "" {
			appVerText = fmt.Sprintf("\n📱 App version: `%s`", pkg.AppVersion)
		}

		msgText := fmt.Sprintf("🆕 *Nueva versión de Helm chart*\nChart: `%s`\nRepo: `%s`\n\n📦 Versión anterior: `%s`\n✅ Versión nueva: `%s`%s",
			chartName, pkg.Repository.Name, oldInfo.Version, pkg.Version, appVerText)

		m := tgbotapi.NewMessage(chatID, msgText)
		m.ParseMode = "Markdown"
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔗 Ver en Artifact Hub", "chart_url:"+chartName),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(m)
	}

	if found == 0 && chatID != 0 {
		sendMessageWithClose(chatID, "✅ Todos los charts trackeados están actualizados")
	}
}

func handleStartContainer(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "exited")),
	})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores detenidos")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "start:"+name1),
		}
		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "start:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "▶️ *Iniciar contenedor*")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleInspectMenu(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📦 Contenedores", "cmd:inspect_containers"),
			tgbotapi.NewInlineKeyboardButtonData("🖼️ Imágenes", "cmd:inspect_images"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💾 Volúmenes", "cmd:inspect_volumes"),
			tgbotapi.NewInlineKeyboardButtonData("🌐 Redes", "cmd:inspect_networks"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar recursos Docker*\n¿Qué deseas inspeccionar?")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleStats(chatID int64) {
	loadingID := sendLoading(chatID, "Recopilando estadísticas del sistema...")
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	dfOut, _ := runCmd("df", "-h", "/")
	memOut, _ := runCmd("free", "-h")

	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	runningContainers, _ := cli.ContainerList(ctx, container.ListOptions{})
	images, _ := cli.ImageList(ctx, image.ListOptions{})
	volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
	networks, _ := cli.NetworkList(ctx, network.ListOptions{})

	diskLines := strings.Split(dfOut, "\n")
	diskInfo := "N/A"
	if len(diskLines) > 1 {
		fields := strings.Fields(diskLines[1])
		if len(fields) >= 5 {
			diskInfo = fmt.Sprintf("%s / %s (%s usado)", fields[2], fields[1], fields[4])
		}
	}

	memLines := strings.Split(memOut, "\n")
	memInfo := "N/A"
	if len(memLines) > 1 {
		fields := strings.Fields(memLines[1])
		if len(fields) >= 3 {
			memInfo = fmt.Sprintf("%s / %s", fields[2], fields[1])
		}
	}

	text := fmt.Sprintf(`📊 *Dashboard de Recursos*

🖥️ *Sistema:*
  • Disco: %s
  • RAM: %s

🐳 *Docker:*
  • Contenedores: %d (%d corriendo)
  • Imágenes: %d
  • Volúmenes: %d
  • Redes: %d`, diskInfo, memInfo, len(containers), len(runningContainers), len(images), len(volumes.Volumes), len(networks))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func recreateContainer(name string) error {
	ctx := context.Background()
	inspect, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect failed: %w", err)
	}

	imageTag := inspect.Config.Image
	reader, err := cli.ImagePull(ctx, imageTag, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	return recreateWithNewImage(name)
}
func getComposeWorkDir(project string) string {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project)),
	})

	if err != nil || len(containers) == 0 {
		log.Printf("No containers found for project: %s", project)
		return ""
	}

	for _, c := range containers {
		inspect, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			continue
		}

		if wd, ok := inspect.Config.Labels["com.docker.compose.project.working_dir"]; ok && wd != "" {
			hostHome := os.Getenv("HOST_HOME")
			if hostHome == "" {
				hostHome = "/home/ubuntu"
			}
			workspace := os.Getenv("WORKSPACE")
			if workspace == "" {
				workspace = "/workspace"
			}

			mappedPath := strings.Replace(wd, hostHome, workspace, 1)

			// Validate directory exists
			if _, err := os.Stat(mappedPath); err != nil {
				log.Printf("Work dir not accessible: %s (mapped from %s)", mappedPath, wd)
				continue
			}

			// Validate compose file exists
			if findComposeFile(mappedPath) == "" {
				log.Printf("No compose file found in: %s", mappedPath)
				continue
			}

			return mappedPath
		}
	}

	log.Printf("No valid working directory found for project: %s", project)
	return ""
}

func handleCompose(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	projectsMap := make(map[string]bool)
	for _, c := range containers {
		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		if project := inspect.Config.Labels["com.docker.compose.project"]; project != "" {
			projectsMap[project] = true
		}
	}

	if len(projectsMap) == 0 {
		sendMessageWithClose(chatID, "No se encontraron proyectos Docker Compose")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for project := range projectsMap {
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📁 "+project, "compose_menu:"+project),
		))
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "📁 *Proyectos Docker Compose*\nSelecciona un proyecto:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handlePrune(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🖼️ Imágenes", "prune_confirm:images"),
			tgbotapi.NewInlineKeyboardButtonData("💾 Volúmenes", "prune_confirm:volumes"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🌐 Redes", "prune_confirm:networks"),
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Todo", "prune_confirm:all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "🗑️ *Limpiar recursos no usados*\n⚠️ Esto eliminará recursos que no están en uso")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleExecMenu(chatID int64) {
	handleGrid(chatID, "⚙️ *Ejecutar comando*\nSelecciona un contenedor:", "exec_menu", false)
}

func handlePauseMenu(chatID int64) {
	handleGrid(chatID, "⏸️ *Pausar contenedor*\nSelecciona un contenedor:", "pause", false)
}

func handleUnpauseMenu(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "paused")),
	})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores pausados")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "unpause:"+name1),
		}
		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "unpause:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "▶️ *Reanudar contenedor*\nSelecciona un contenedor:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}
func handleFavorites(chatID int64, userID int64) {
	favs := favorites[userID]
	if len(favs) == 0 {
		sendMessageWithClose(chatID, "No tienes favoritos.\nUsa /addfav para agregar.")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for _, name := range favs {
		icon := getIcon(name)
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(icon+" "+name, "fav_action:"+name),
		))
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "⭐ *Tus favoritos*")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleAddFavorite(chatID int64, userID int64, containerName string) {
	if containerName == "" {
		return
	}

	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", containerName)),
	})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, fmt.Sprintf("❌ Contenedor *%s* no encontrado", containerName))
		return
	}

	for _, fav := range favorites[userID] {
		if fav == containerName {
			sendMessageWithClose(chatID, fmt.Sprintf("⭐ *%s* ya está en favoritos", containerName))
			return
		}
	}

	favorites[userID] = append(favorites[userID], containerName)
	sendMessageWithClose(chatID, fmt.Sprintf("✅ *%s* agregado a favoritos", containerName))
}

func handleAddFavoriteMenu(chatID int64, userID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		icon1 := getIcon(name1)
		isFav1 := false
		for _, fav := range favorites[userID] {
			if fav == name1 {
				isFav1 = true
				break
			}
		}

		label1 := icon1 + " " + name1
		if isFav1 {
			label1 = "✅ " + label1
		}

		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label1, "togglefav:"+name1),
		}

		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			icon2 := getIcon(name2)
			isFav2 := false
			for _, fav := range favorites[userID] {
				if fav == name2 {
					isFav2 = true
					break
				}
			}

			label2 := icon2 + " " + name2
			if isFav2 {
				label2 = "✅ " + label2
			}

			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label2, "togglefav:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "⭐ *Agregar/Quitar Favoritos*\nSelecciona contenedores (✅ = en favoritos):")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleEnvMenu(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores corriendo")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := strings.TrimPrefix(containers[i].Names[0], "/")
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "env:"+name1),
		}
		if i+1 < len(containers) {
			name2 := strings.TrimPrefix(containers[i+1].Names[0], "/")
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "env:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "🔧 *Variables de entorno*\nSelecciona un contenedor:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleHistory(chatID int64, userID int64) {
	history := commandHistory[userID]
	if len(history) == 0 {
		sendMessageWithClose(chatID, "No hay historial de comandos")
		return
	}

	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}

	text := "📜 *Historial de comandos*\n\n"
	for i := start; i < len(history); i++ {
		text += fmt.Sprintf("%d. /%s\n", i-start+1, history[i])
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}
func handleCreateMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "🐳 *Crear nuevo contenedor*\n\n¿Cómo deseas crear el contenedor?")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📦 Docker Run", "create_type:run"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🐙 Docker Compose", "create_type:compose"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func handleCreateRun(chatID int64, userID int64) {
	createData[userID] = make(map[string]string)
	createData[userID]["type"] = "run"
	userState[userID] = "create_image"
	sendMessageWithClose(chatID, "📦 *Crear contenedor con Docker Run*\n\n1️⃣ Escribe el nombre de la imagen:\nEjemplo: `nginx:latest`, `postgres:15`")
}

func handleCreateCompose(chatID int64, userID int64) {
	createData[userID] = make(map[string]string)
	createData[userID]["type"] = "compose"
	userState[userID] = "create_service_name"
	sendMessageWithClose(chatID, "🐙 *Crear contenedor con Docker Compose*\n\n1️⃣ Escribe el nombre del servicio:\nEjemplo: `web`, `database`")
}

func processCreateStep(chatID int64, userID int64, text string) {
	state := userState[userID]
	data := createData[userID]

	switch state {
	case "create_image":
		data["image"] = text
		userState[userID] = "create_name"
		sendMessageWithClose(chatID, "2️⃣ Escribe el nombre del contenedor:\nEjemplo: `mi-nginx`\n\n_Presiona /skip para generar automáticamente_")
	case "create_name":
		if text != "/skip" {
			data["name"] = text
		}
		userState[userID] = "create_ports"
		sendMessageWithClose(chatID, "3️⃣ Escribe los puertos (opcional):\nEjemplo: `80:80`, `8080:80,3306:3306`\n\n_Presiona /skip para omitir_")
	case "create_ports":
		if text != "/skip" {
			data["ports"] = text
		}
		userState[userID] = "create_volumes"
		sendMessageWithClose(chatID, "4️⃣ Escribe los volúmenes (opcional):\nEjemplo: `/data:/app/data`\n\n_Presiona /skip para omitir_")
	case "create_volumes":
		if text != "/skip" {
			data["volumes"] = text
		}
		userState[userID] = "create_env"
		sendMessageWithClose(chatID, "5️⃣ Escribe las variables de entorno (opcional):\nEjemplo: `DB_USER=admin,DB_PASS=secret`\n\n_Presiona /skip para omitir_")
	case "create_env":
		if text != "/skip" {
			data["env"] = text
		}
		delete(userState, userID)
		generateDockerRun(chatID, userID)
	case "create_service_name":
		data["service"] = text
		userState[userID] = "create_compose_image"
		sendMessageWithClose(chatID, "2️⃣ Escribe el nombre de la imagen:\nEjemplo: `nginx:latest`")
	case "create_compose_image":
		data["image"] = text
		userState[userID] = "create_compose_ports"
		sendMessageWithClose(chatID, "3️⃣ Escribe los puertos (opcional):\nEjemplo: `80:80`\n\n_Presiona /skip para omitir_")
	case "create_compose_ports":
		if text != "/skip" {
			data["ports"] = text
		}
		userState[userID] = "create_compose_volumes"
		sendMessageWithClose(chatID, "4️⃣ Escribe los volúmenes (opcional):\nEjemplo: `/data:/app/data`\n\n_Presiona /skip para omitir_")
	case "create_compose_volumes":
		if text != "/skip" {
			data["volumes"] = text
		}
		userState[userID] = "create_compose_env"
		sendMessageWithClose(chatID, "5️⃣ Escribe las variables de entorno (opcional):\nEjemplo: `DB_USER=admin`\n\n_Presiona /skip para omitir_")
	case "create_compose_env":
		if text != "/skip" {
			data["env"] = text
		}
		delete(userState, userID)
		generateDockerCompose(chatID, userID)
	}
}

func generateDockerRun(chatID int64, userID int64) {
	data := createData[userID]
	cmd := "docker run -d"
	if name, ok := data["name"]; ok {
		cmd += " --name " + name
	}
	if ports, ok := data["ports"]; ok {
		for _, port := range strings.Split(ports, ",") {
			cmd += " -p " + strings.TrimSpace(port)
		}
	}
	if volumes, ok := data["volumes"]; ok {
		for _, vol := range strings.Split(volumes, ",") {
			cmd += " -v " + strings.TrimSpace(vol)
		}
	}
	if env, ok := data["env"]; ok {
		for _, e := range strings.Split(env, ",") {
			cmd += " -e " + strings.TrimSpace(e)
		}
	}
	cmd += " " + data["image"]

	text := fmt.Sprintf("✅ *Comando generado:*\n\n```bash\n%s\n```\n\n¿Deseas ejecutarlo ahora?", cmd)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Ejecutar", "create_exec:"+cmd),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
		),
	)
	bot.Send(msg)
	delete(createData, userID)
}

func generateDockerCompose(chatID int64, userID int64) {
	data := createData[userID]
	compose := fmt.Sprintf("services:\n  %s:\n    image: %s\n    container_name: %s\n    restart: unless-stopped",
		data["service"], data["image"], data["service"])

	if ports, ok := data["ports"]; ok {
		compose += "\n    ports:"
		for _, port := range strings.Split(ports, ",") {
			compose += fmt.Sprintf("\n      - \"%s\"", strings.TrimSpace(port))
		}
	}
	if volumes, ok := data["volumes"]; ok {
		compose += "\n    volumes:"
		for _, vol := range strings.Split(volumes, ",") {
			compose += fmt.Sprintf("\n      - %s", strings.TrimSpace(vol))
		}
	}
	if env, ok := data["env"]; ok {
		compose += "\n    environment:"
		for _, e := range strings.Split(env, ",") {
			parts := strings.SplitN(strings.TrimSpace(e), "=", 2)
			if len(parts) == 2 {
				compose += fmt.Sprintf("\n      %s: %s", parts[0], parts[1])
			}
		}
	}

	text := fmt.Sprintf("✅ *Docker Compose generado:*\n\n```yaml\n%s\n```\n\nGuarda este contenido en `docker-compose.yml` y ejecuta:\n```bash\ndocker compose up -d\n```", compose)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
	delete(createData, userID)
}
func handleDiagnose(chatID int64) {
	loadingID := sendLoading(chatID, "Ejecutando diagnóstico...")
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	issues := []string{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Check 1: Stopped containers
	wg.Add(1)
	go func() {
		defer wg.Done()
		stoppedContainers, err := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("status", "exited")),
		})
		if err == nil && len(stoppedContainers) > 0 {
			mu.Lock()
			issues = append(issues, fmt.Sprintf("⚠️ %d contenedores detenidos", len(stoppedContainers)))
			mu.Unlock()
		}
	}()

	// Check 2: High CPU usage
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats := getStats()
		for name, stat := range stats {
			var cpu float64
			fmt.Sscanf(strings.TrimSuffix(stat.CPU, "%"), "%f", &cpu)
			if cpu > 80 {
				mu.Lock()
				issues = append(issues, fmt.Sprintf("🔥 %s usando %s CPU", name, stat.CPU))
				mu.Unlock()
			}
		}
	}()

	// Check 3: Dangling images
	wg.Add(1)
	go func() {
		defer wg.Done()
		danglingImages, err := cli.ImageList(ctx, image.ListOptions{
			Filters: filters.NewArgs(filters.Arg("dangling", "true")),
		})
		if err == nil && len(danglingImages) > 0 {
			mu.Lock()
			issues = append(issues, fmt.Sprintf("🗑️ %d imágenes sin usar (ejecuta /prune)", len(danglingImages)))
			mu.Unlock()
		}
	}()

	wg.Wait()

	if len(issues) == 0 {
		sendMessageWithClose(chatID, "✅ *Todo está bien*\nNo se detectaron problemas en el sistema")
	} else {
		text := fmt.Sprintf("🔍 *Diagnóstico del sistema*\n_%d problema(s) detectado(s)_\n\n%s\n\n💡 Usa /list para gestionar contenedores o /prune para limpiar recursos.", len(issues), strings.Join(issues, "\n"))
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Prune", "cmd:prune_menu"),
				tgbotapi.NewInlineKeyboardButtonData("📋 Lista", "cmd:list"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
	}
}

func handleUptime(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores corriendo")
		return
	}

	text := "⏱️ *Uptime de contenedores*\n\n"
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		icon := getIcon(name)
		text += fmt.Sprintf("%s *%s*\n   └ `%s`\n", icon, name, c.Status)
	}
	sendMessageWithClose(chatID, text)
}

func handleBackupMenu(chatID int64) {
	ctx := context.Background()
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil || len(volumes.Volumes) == 0 {
		sendMessageWithClose(chatID, "No hay volúmenes disponibles")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(volumes.Volumes); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("💾 "+volumes.Volumes[i].Name, "backup:"+volumes.Volumes[i].Name),
		}
		if i+1 < len(volumes.Volumes) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("💾 "+volumes.Volumes[i+1].Name, "backup:"+volumes.Volumes[i+1].Name))
		}
		keyboard = append(keyboard, row)
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "💾 *Backup de volumen*\nSelecciona el volumen a exportar:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN no configurado")
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Docker client
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("Error connecting to Docker:", err)
	}
	defer cli.Close()

	// Start API server for Mini App
	apiServer := api.NewServer(cli)
	go func() {
		if err := apiServer.Start("8080"); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	// Validate Docker Compose availability
	if err := validateComposeSetup(); err != nil {
		log.Printf("⚠️  Warning: %v", err)
		log.Printf("⚠️  Compose features will be disabled")
	}

	log.Printf("Bot iniciado: @%s", bot.Self.UserName)

	// Load language
	if lang := os.Getenv("LANGUAGE"); lang != "" {
		language = strings.ToLower(lang)
	}
	if err := loadLanguage(language); err != nil {
		log.Printf("⚠️  Warning: Could not load language '%s', using defaults: %v", language, err)
		// Try loading Spanish as fallback
		if language != "es" {
			language = "es"
			if err := loadLanguage(language); err != nil {
				log.Fatal("Could not load default language (es):", err)
			}
		}
	}
	log.Printf("Language loaded: %s", language)

	// Load configuration from file
	loadConfig()

	// Load configuration from environment variables
	if intervalStr := os.Getenv("CHECK_UPDATES_INTERVAL"); intervalStr != "" {
		var hours int
		if _, err := fmt.Sscanf(intervalStr, "%d", &hours); err == nil && hours > 0 {
			checkUpdatesInterval = time.Duration(hours) * time.Hour
			log.Printf("Check updates interval: %d hours", hours)
		}
	}

	if autoCheckStr := os.Getenv("ENABLE_AUTO_CHECK"); autoCheckStr != "" {
		enableAutoCheck = autoCheckStr == "true"
		log.Printf("Auto-check enabled: %v", enableAutoCheck)
	}

	if startupNotifStr := os.Getenv("ENABLE_STARTUP_NOTIFICATION"); startupNotifStr != "" {
		enableStartupNotif = startupNotifStr == "true"
	}

	// Load allowed users
	if usersStr := os.Getenv("ALLOWED_USERS"); usersStr != "" {
		for _, idStr := range strings.Split(usersStr, ",") {
			var userID int64
			if _, err := fmt.Sscanf(strings.TrimSpace(idStr), "%d", &userID); err == nil {
				allowedUsers = append(allowedUsers, userID)
			}
		}
		log.Printf("Allowed users: %v", allowedUsers)
	}

	// Load notify chat ID
	if chatIDStr := os.Getenv("NOTIFY_CHAT_ID"); chatIDStr != "" {
		fmt.Sscanf(strings.TrimSpace(chatIDStr), "%d", &notifyChatID)
		log.Printf("Notify chat ID loaded: %d", notifyChatID)
	}

	// Send startup notification
	if enableStartupNotif && notifyChatID != 0 {
		startupMsg := getText("bot_started", botVersion)
		msg := tgbotapi.NewMessage(notifyChatID, startupMsg)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL(getText("button_news_channel"), newsChannelURL),
			),
		)
		bot.Send(msg)
	}

	// Set bot commands
	commands := []tgbotapi.BotCommand{
		// Menu & Status
		{Command: "start", Description: getText("menu_start")},
		{Command: "list", Description: getText("menu_list")},
		{Command: "ps", Description: getText("menu_ps")},
		{Command: "running", Description: getText("menu_running")},
		{Command: "stats", Description: getText("menu_stats")},
		{Command: "uptime", Description: getText("menu_uptime")},
		
		// Container Management
		{Command: "create", Description: getText("menu_create")},
		{Command: "restart", Description: getText("menu_restart")},
		{Command: "stop", Description: getText("menu_stop")},
		{Command: "start_container", Description: getText("menu_start_container")},
		{Command: "pause", Description: getText("menu_pause")},
		{Command: "unpause", Description: getText("menu_unpause")},
		{Command: "logs", Description: getText("menu_logs")},
		{Command: "logfile", Description: getText("menu_logfile")},
		{Command: "exec", Description: getText("menu_exec")},
		{Command: "env", Description: getText("menu_env")},
		{Command: "inspect", Description: getText("menu_inspect")},
		
		// Docker Compose
		{Command: "compose", Description: getText("menu_compose")},
		
		// Images & Updates
		{Command: "images", Description: getText("menu_images")},
		{Command: "checkupdates", Description: getText("menu_checkupdates")},
		{Command: "autoupdate", Description: getText("menu_autoupdate")},
		{Command: "trackimage", Description: getText("menu_trackimage")},
		{Command: "trackchart", Description: getText("menu_trackchart")},
		
		// Resources
		{Command: "volumes", Description: getText("menu_volumes")},
		{Command: "networks", Description: getText("menu_networks")},
		{Command: "prune", Description: getText("menu_prune")},
		
		// Utilities
		{Command: "diagnose", Description: getText("menu_diagnose")},
		{Command: "search", Description: getText("menu_search")},
		{Command: "favorites", Description: getText("menu_favorites")},
		{Command: "addfav", Description: getText("menu_addfav")},
		{Command: "history", Description: getText("menu_history")},
		{Command: "backup", Description: getText("menu_backup")},
		{Command: "version", Description: getText("menu_version")},
		// Phase 2
		{Command: "rollback", Description: "↩️ Rollback a imagen anterior"},
		{Command: "templates", Description: "📋 Gestionar plantillas de contenedores"},
		{Command: "maintenance", Description: "🔧 Modo mantenimiento"},
		// Phase 1
		{Command: "alerts", Description: "⚠️ Alertas de recursos"},
		{Command: "healthchecks", Description: "🏥 Health checks"},
		{Command: "reports", Description: "📊 Reportes programados"},
		// Phase 3
		{Command: "audit", Description: "📋 Registro de auditoría"},
		{Command: "scan", Description: "🔒 Escanear vulnerabilidades"},
		{Command: "webhooks", Description: "🔗 Gestionar webhooks"},
		{Command: "policies", Description: "⚙️ Políticas de actualización"},
		// Phase 4
		{Command: "networks", Description: "🌐 Gestionar redes"},
		{Command: "cleanup", Description: "🧹 Limpieza inteligente"},
		{Command: "ports", Description: "🔌 Gestión de puertos"},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(cmdConfig); err != nil {
		log.Printf("Error setting commands: %v", err)
	}

	go monitorEvents()
	go checkUpdates()
	go monitorResources()
	go runHealthChecks()
	go sendScheduledReports()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			chatID := update.Message.Chat.ID
			userID := update.Message.From.ID
			notifyChatID = chatID

			// Check authentication
			if len(allowedUsers) > 0 {
				allowed := false
				for _, id := range allowedUsers {
					if id == userID {
						allowed = true
						break
					}
				}
				if !allowed {
					sendMessageWithClose(chatID, "❌ No autorizado")
					continue
				}
			}

			// Log command
			if update.Message.Command() != "" {
				commandHistory[userID] = append(commandHistory[userID], update.Message.Command())
				if len(commandHistory[userID]) > 50 {
					commandHistory[userID] = commandHistory[userID][1:]
				}
				// Add to audit log
				addAudit(userID, update.Message.Command(), update.Message.CommandArguments(), true)
			}

			// Delete command message
			bot.Send(tgbotapi.NewDeleteMessage(chatID, update.Message.MessageID))

			// Check user state
			if state, exists := userState[userID]; exists && update.Message.Command() == "" {
				text := update.Message.Text
				if strings.HasPrefix(state, "create_") {
					go processCreateStep(chatID, userID, text)
					continue
				}
				if state == "waiting_search" {
					delete(userState, userID)
					go handleSearch(chatID, text)
					continue
				}
				if state == "waiting_track_image" {
					delete(userState, userID)
					go addTrackedImage(chatID, text)
					continue
				}
				if state == "waiting_track_chart" {
					delete(userState, userID)
					go addTrackedChart(chatID, text)
					continue
				}
				if state == "webhook_name" {
					if createData[userID] == nil {
						createData[userID] = make(map[string]string)
					}
					createData[userID]["webhook_name"] = text
					userState[userID] = "webhook_url"
					sendMessageWithClose(chatID, "🔗 Escribe la URL del webhook:")
					continue
				}
				if state == "webhook_url" {
					createData[userID]["webhook_url"] = text
					userState[userID] = "webhook_events"
					keyboard := tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("🟢 Start", "wh_event:start"),
							tgbotapi.NewInlineKeyboardButtonData("🔴 Stop", "wh_event:stop"),
						),
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("💥 Die", "wh_event:die"),
							tgbotapi.NewInlineKeyboardButtonData("🔄 Update", "wh_event:update"),
						),
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("✅ Todos", "wh_event:all"),
						),
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("💾 Guardar", "wh_save"),
						),
					)
					msg := tgbotapi.NewMessage(chatID, "📋 Selecciona los eventos (puedes elegir varios):")
					msg.ReplyMarkup = keyboard
					bot.Send(msg)
					continue
				}
			}

			switch update.Message.Command() {
			case "start":
				go handleStart(chatID)
			case "version":
				go checkBotVersion(chatID)
			case "ps":
				go handlePS(chatID)
			case "running":
				go handleRunning(chatID)
			case "list":
				go handleList(chatID)
			case "restart":
				go handleGrid(chatID, "🔄 *Reiniciar contenedor*", "restart", false)
			case "stop":
				go handleGrid(chatID, "⏸️ *Detener contenedor*", "stop", false)
			case "logs":
				go handleGrid(chatID, "📊 *Ver logs*", "logs", false)
			case "logfile":
				go handleGrid(chatID, "💾 *Descargar logs*", "logfile", false)
			case "create":
				go handleCreateMenu(chatID)
			case "images":
				go handleImages(chatID)
			case "volumes":
				go handleVolumes(chatID)
			case "networks":
				go handleNetworks(chatID)
			case "start_container":
				go handleStartContainer(chatID)
			case "inspect":
				go handleInspectMenu(chatID)
			case "stats":
				go handleStats(chatID)
			case "compose":
				go handleCompose(chatID)
			case "prune":
				go handlePrune(chatID)
			case "exec":
				go handleExecMenu(chatID)
			case "pause":
				go handlePauseMenu(chatID)
			case "unpause":
				go handleUnpauseMenu(chatID)
			case "favorites":
				go handleFavorites(chatID, userID)
			case "addfav":
				if update.Message.CommandArguments() != "" {
					go handleAddFavorite(chatID, userID, update.Message.CommandArguments())
				} else {
					go handleAddFavoriteMenu(chatID, userID)
				}
			case "env":
				go handleEnvMenu(chatID)
			case "history":
				go handleHistory(chatID, userID)
			case "diagnose":
				go handleDiagnose(chatID)
			case "checkupdates":
				go func() {
					sendMessageWithClose(chatID, "🔍 Buscando actualizaciones...")
					runImageUpdateCheckWithFeedback(chatID)
				}()
			case "autoupdate":
				go handleAutoUpdate(chatID)
			case "trackimage":
				go handleTrackImage(chatID)
			case "trackchart":
				go handleTrackChart(chatID)
			case "uptime":
				go handleUptime(chatID)
			case "backup":
				go handleBackupMenu(chatID)
			// Phase 2
			case "rollback":
				go handleRollback(chatID)
			case "templates":
				go handleTemplates(chatID)
			case "maintenance":
				go handleMaintenance(chatID)
			case "search":
				if update.Message.CommandArguments() == "" {
					userState[userID] = "waiting_search"
					sendMessageWithClose(chatID, "🔍 ¿Qué deseas buscar?\n\n_Filtros disponibles:_\n• `label:key=value` — por etiqueta\n• `env:VAR` — por variable de entorno\n• `status:running` — por estado\n• Texto libre — nombre o imagen")
				} else {
					go handleSearch(chatID, update.Message.CommandArguments())
				}
			// Phase 1
			case "alerts":
				go handleAlerts(chatID)
			case "healthchecks":
				go handleHealthChecks(chatID)
			case "reports":
				go handleReports(chatID)
			// Phase 3
			case "audit":
				go handleAudit(chatID)
			case "scan":
				go handleScan(chatID)
			case "webhooks":
				go handleWebhooks(chatID)
			case "policies":
				go handlePolicies(chatID)
			// Phase 4
			case "cleanup":
				go handleCleanup(chatID)
			case "ports":
				go handlePorts(chatID)
			}
		} else if update.CallbackQuery != nil {
			go handleCallback(update.CallbackQuery)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 2: Rollback System
// ═══════════════════════════════════════════════════════════════════════════

func saveRollbackEntry(containerName, imageTag, imageID string) {
	configMutex.Lock()
	defer configMutex.Unlock()
	entry := RollbackEntry{Image: imageTag, ImageID: imageID, Timestamp: time.Now()}
	history := rollbackHistory[containerName]
	// Prepend (newest first), keep max 5
	history = append([]RollbackEntry{entry}, history...)
	if len(history) > 5 {
		history = history[:5]
	}
	rollbackHistory[containerName] = history
}

func doRollback(containerName string, entry RollbackEntry) error {
	ctx := context.Background()
	inspect, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect failed: %w", err)
	}

	// Pull the old image
	reader, err := cli.ImagePull(ctx, entry.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	wasRunning := inspect.State.Running
	timeout := 10
	cli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: &timeout})

	oldName := containerName + "_rollback_old"
	cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true})
	if err := cli.ContainerRename(ctx, containerName, oldName); err != nil {
		if wasRunning {
			cli.ContainerStart(ctx, containerName, container.StartOptions{})
		}
		return fmt.Errorf("rename failed: %w", err)
	}

	cfg := inspect.Config
	cfg.Image = entry.Image
	resp, err := cli.ContainerCreate(ctx, cfg, inspect.HostConfig, &network.NetworkingConfig{
		EndpointsConfig: inspect.NetworkSettings.Networks,
	}, nil, containerName)
	if err != nil {
		cli.ContainerRename(ctx, oldName, containerName)
		if wasRunning {
			cli.ContainerStart(ctx, containerName, container.StartOptions{})
		}
		return fmt.Errorf("create failed: %w", err)
	}

	if wasRunning {
		if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})
			cli.ContainerRename(ctx, oldName, containerName)
			cli.ContainerStart(ctx, containerName, container.StartOptions{})
			return fmt.Errorf("start failed: %w", err)
		}
	}
	cli.ContainerRemove(ctx, oldName, container.RemoveOptions{Force: true})
	return nil
}

func handleRollback(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	// Only show containers that have rollback history
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		history := rollbackHistory[name]
		if len(history) == 0 {
			continue
		}
		icon := getIcon(name)
		label := fmt.Sprintf("%s %s (%d versiones)", icon, name, len(history))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "rollback_container:"+name),
			tgbotapi.NewInlineKeyboardButtonData("🗑️", "rollback_clear:"+name),
		))
	}

	if len(rows) == 0 {
		sendMessageWithClose(chatID, "↩️ *Rollback*\n\nNo hay historial de versiones.\nEl historial se guarda automáticamente cuando se actualiza un contenedor.")
		return
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))
	msg := tgbotapi.NewMessage(chatID, "↩️ *Rollback de contenedores*\nSelecciona un contenedor para revertir:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 2: Container Templates
// ═══════════════════════════════════════════════════════════════════════════

func saveTemplate(containerName string) error {
	ctx := context.Background()
	inspect, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect failed: %w", err)
	}

	tpl := ContainerTemplate{
		Name:        containerName,
		Image:       inspect.Config.Image,
		Cmd:         inspect.Config.Cmd,
		Env:         inspect.Config.Env,
		Labels:      inspect.Config.Labels,
		NetworkMode: string(inspect.HostConfig.NetworkMode),
		CreatedAt:   time.Now(),
	}

	// Extract port bindings
	if inspect.HostConfig.PortBindings != nil {
		tpl.Ports = make(map[string]string)
		for containerPort, bindings := range inspect.HostConfig.PortBindings {
			for _, b := range bindings {
				tpl.Ports[b.HostPort] = string(containerPort)
			}
		}
	}

	// Extract volume bindings
	for _, bind := range inspect.HostConfig.Binds {
		tpl.Volumes = append(tpl.Volumes, bind)
	}

	if inspect.HostConfig.RestartPolicy.Name != "" {
		tpl.RestartPolicy = string(inspect.HostConfig.RestartPolicy.Name)
	}

	configMutex.Lock()
	templates[containerName] = tpl
	configMutex.Unlock()
	saveConfig()
	return nil
}

func deployTemplate(tpl ContainerTemplate) error {
	ctx := context.Background()

	// Pull image
	reader, err := cli.ImagePull(ctx, tpl.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	cfg := &container.Config{
		Image:  tpl.Image,
		Cmd:    tpl.Cmd,
		Env:    tpl.Env,
		Labels: tpl.Labels,
	}

	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(tpl.NetworkMode),
		Binds:       tpl.Volumes,
	}
	if tpl.RestartPolicy != "" {
		hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(tpl.RestartPolicy)}
	}

	// Remove existing container with same name if stopped
	existing, err := cli.ContainerInspect(ctx, tpl.Name)
	if err == nil && !existing.State.Running {
		cli.ContainerRemove(ctx, tpl.Name, container.RemoveOptions{})
	}

	resp, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, tpl.Name)
	if err != nil {
		return fmt.Errorf("create failed: %w", err)
	}
	return cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
}

func handleTemplates(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	text := "📋 *Plantillas de contenedores*\n\n"
	var rows [][]tgbotapi.InlineKeyboardButton

	if len(templates) > 0 {
		text += fmt.Sprintf("Guardadas: %d\n\n", len(templates))
		for name := range templates {
			icon := getIcon(name)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(icon+" "+name, "tpl_info:"+name),
			))
		}
	} else {
		text += "No hay plantillas guardadas.\n"
	}

	// Add "save from container" section
	if len(containers) > 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💾 Guardar contenedor como plantilla", "tpl_save_menu:_"),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 2: Advanced Search
// ═══════════════════════════════════════════════════════════════════════════

func handleSearch(chatID int64, query string) {
	if query == "" {
		return
	}

	ctx := context.Background()
	queryLower := strings.ToLower(strings.TrimSpace(query))
	results := []string{}

	// Parse filter prefix
	filterType := ""
	filterValue := ""
	if strings.HasPrefix(queryLower, "label:") {
		filterType = "label"
		filterValue = strings.TrimPrefix(queryLower, "label:")
	} else if strings.HasPrefix(queryLower, "env:") {
		filterType = "env"
		filterValue = strings.TrimPrefix(queryLower, "env:")
	} else if strings.HasPrefix(queryLower, "status:") {
		filterType = "status"
		filterValue = strings.TrimPrefix(queryLower, "status:")
	}

	var listOpts container.ListOptions
	if filterType == "status" {
		listOpts = container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("status", filterValue)),
		}
	} else {
		listOpts = container.ListOptions{All: true}
	}

	containers, _ := cli.ContainerList(ctx, listOpts)

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		matched := false

		switch filterType {
		case "label":
			// Search by label key=value or just key
			inspect, _ := cli.ContainerInspect(ctx, c.ID)
			for k, v := range inspect.Config.Labels {
				labelStr := strings.ToLower(k + "=" + v)
				if strings.Contains(labelStr, filterValue) {
					matched = true
					break
				}
			}
		case "env":
			inspect, _ := cli.ContainerInspect(ctx, c.ID)
			for _, e := range inspect.Config.Env {
				if strings.Contains(strings.ToLower(e), filterValue) {
					matched = true
					break
				}
			}
		case "status":
			matched = true // already filtered by Docker
		default:
			// Free text: match name or image
			if strings.Contains(strings.ToLower(name), queryLower) ||
				strings.Contains(strings.ToLower(c.Image), queryLower) {
				matched = true
			}
		}

		if matched {
			statusIcon := "🔴"
			if c.State == "running" {
				statusIcon = "🟢"
			} else if c.State == "paused" {
				statusIcon = "🟡"
			}
			results = append(results, fmt.Sprintf("%s %s *%s*\n   └ `%s`", statusIcon, getIcon(name), name, c.Image))
		}
	}

	// Also search images (only for free text)
	if filterType == "" {
		images, _ := cli.ImageList(ctx, image.ListOptions{})
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if strings.Contains(strings.ToLower(tag), queryLower) {
					results = append(results, fmt.Sprintf("🖼️ `%s`", tag))
					break
				}
			}
		}

		vols, _ := cli.VolumeList(ctx, volume.ListOptions{})
		for _, vol := range vols.Volumes {
			if strings.Contains(strings.ToLower(vol.Name), queryLower) {
				results = append(results, fmt.Sprintf("💾 `%s`", vol.Name))
			}
		}
	}

	if len(results) == 0 {
		sendMessageWithClose(chatID, fmt.Sprintf("🔍 Sin resultados para: `%s`", query))
		return
	}

	text := fmt.Sprintf("🔍 *Resultados para:* `%s`\n_%d encontrado(s)_\n\n%s", query, len(results), strings.Join(results, "\n\n"))
	if len(text) > 3800 {
		text = text[:3800] + "\n...(truncado)"
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 2: Maintenance Mode
// ═══════════════════════════════════════════════════════════════════════════

// criticalContainers are never paused during maintenance
func activateMaintenance() (int, error) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	if err != nil {
		return 0, err
	}

	paused := []string{}
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		if criticalContainers[name] {
			continue
		}
		if err := cli.ContainerPause(ctx, c.ID); err == nil {
			paused = append(paused, name)
		}
	}

	configMutex.Lock()
	maintenanceMode = true
	maintenancePaused = paused
	configMutex.Unlock()
	saveConfig()
	return len(paused), nil
}

func deactivateMaintenance() (int, error) {
	ctx := context.Background()
	count := 0
	for _, name := range maintenancePaused {
		if err := cli.ContainerUnpause(ctx, name); err == nil {
			count++
		}
	}

	configMutex.Lock()
	maintenanceMode = false
	maintenancePaused = nil
	configMutex.Unlock()
	saveConfig()
	return count, nil
}

func handleMaintenance(chatID int64) {
	ctx := context.Background()
	running, _ := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	paused, _ := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "paused")),
	})

	statusIcon := "🟢"
	statusText := "Inactivo"
	if maintenanceMode {
		statusIcon = "🔧"
		statusText = "ACTIVO"
	}

	text := fmt.Sprintf("🔧 *Modo Mantenimiento*\n\nEstado: %s *%s*\n\n🟢 Corriendo: %d\n⏸️ Pausados: %d",
		statusIcon, statusText, len(running), len(paused))

	if maintenanceMode && len(maintenancePaused) > 0 {
		text += fmt.Sprintf("\n\n_Pausados por mantenimiento:_\n`%s`", strings.Join(maintenancePaused, "`, `"))
	}

	var keyboard tgbotapi.InlineKeyboardMarkup
	if maintenanceMode {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Desactivar mantenimiento", "maintenance_off:_"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔄 Actualizar estado", "maintenance_status:_"),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
	} else {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔧 Activar mantenimiento", "maintenance_on:_"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔄 Actualizar estado", "maintenance_status:_"),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// ═══════════════════════════════════════════════════════════════════════════
// Newer Tag Detection
// ═══════════════════════════════════════════════════════════════════════════

var knownSuffixes = []string{
	"-alpine3.21", "-alpine3.20", "-alpine3.19", "-alpine3.18", "-alpine",
	"-slim-bookworm", "-bookworm", "-slim-bullseye", "-bullseye", "-slim",
	"-perl", "-otel", "-windowsservercore", "-nanoserver",
}

var skipTags = map[string]bool{
	"latest": true, "stable": true, "edge": true, "nightly": true,
	"develop": true, "main": true, "master": true, "lts": true, "mainline": true,
}

var (
	registryTokenCache      = make(map[string]string)
	registryTokenCacheMutex sync.Mutex
)

// tagParts splits "1.25.0-alpine" into version="1.25.0", suffix="-alpine"
func tagParts(tag string) (version, suffix string) {
	for _, s := range knownSuffixes {
		if strings.HasSuffix(tag, s) {
			return strings.TrimSuffix(tag, s), s
		}
	}
	return tag, ""
}

// parseRegistryAndRepo extracts registry and repo from image name
// Examples:
//   nginx → registry-1.docker.io, library/nginx
//   user/image → registry-1.docker.io, user/image
//   ghcr.io/user/image → ghcr.io, user/image
func parseRegistryAndRepo(image string) (registry, repo string) {
	parts := strings.Split(image, "/")
	
	if len(parts) == 1 {
		// Official image: nginx → library/nginx
		return "registry-1.docker.io", "library/" + parts[0]
	}
	
	if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
		// Has registry: ghcr.io/user/image
		return parts[0], strings.Join(parts[1:], "/")
	}
	
	// User image: user/image
	return "registry-1.docker.io", image
}

// fetchRegistryToken gets a Bearer token for registry API access
func fetchRegistryToken(registry, repo string) (string, error) {
	var authURL string
	
	if registry == "registry-1.docker.io" {
		authURL = fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
	} else if registry == "ghcr.io" {
		authURL = fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", repo)
	} else {
		// For other registries, try to discover auth endpoint
		return "", fmt.Errorf("unsupported registry: %s", registry)
	}
	
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(authURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	return result.Token, nil
}

// listRegistryTags fetches available tags from registry
func listRegistryTags(registry, repo, token string) ([]string, error) {
	tagsURL := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repo)
	
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", tagsURL, nil)
	if err != nil {
		return nil, err
	}
	
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}
	
	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	return result.Tags, nil
}

// findNewerTag checks if a newer version of the same tag variant exists
// Example: alpine:3.18 → finds alpine:3.20 if available
func findNewerTag(imageTag string) (string, error) {
	// Split image:tag
	parts := strings.Split(imageTag, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image format")
	}
	image, currentTag := parts[0], parts[1]
	
	// Skip floating tags
	if skipTags[currentTag] {
		return "", nil
	}
	
	// Extract version and suffix
	currentVer, currentSuffix := tagParts(currentTag)
	
	// Try to parse as semver
	cv, err := semver.NewVersion(currentVer)
	if err != nil {
		// Not a semver tag, skip
		return "", nil
	}
	
	// Get registry and repo
	registry, repo := parseRegistryAndRepo(image)
	
	// Fetch token (with cache)
	cacheKey := registry + ":" + repo
	registryTokenCacheMutex.Lock()
	token, cached := registryTokenCache[cacheKey]
	registryTokenCacheMutex.Unlock()
	
	if !cached {
		var err error
		token, err = fetchRegistryToken(registry, repo)
		if err != nil {
			return "", nil // Silent fail
		}
		registryTokenCacheMutex.Lock()
		registryTokenCache[cacheKey] = token
		registryTokenCacheMutex.Unlock()
	}
	
	// List tags
	allTags, err := listRegistryTags(registry, repo, token)
	if err != nil {
		return "", nil // Silent fail
	}
	
	// Find best newer tag with same suffix
	var best *semver.Version
	var bestTag string
	
	// Determine if current version is major.minor or major.minor.patch
	currentParts := strings.Split(cv.String(), ".")
	
	for _, tag := range allTags {
		// Skip floating tags
		if skipTags[tag] {
			continue
		}
		
		ver, suffix := tagParts(tag)
		
		// Must have same suffix
		if suffix != currentSuffix {
			continue
		}
		
		v, err := semver.NewVersion(ver)
		if err != nil {
			continue // Not parseable
		}
		
		// Skip pre-releases
		if v.Prerelease() != "" {
			continue
		}
		
		// Only compare versions with similar structure (e.g., 3.18 vs 3.21, not 3.18 vs 20260127)
		candidateParts := strings.Split(v.String(), ".")
		if len(candidateParts) != len(currentParts) {
			continue
		}
		
		// Skip if major version is drastically different (likely a date-based tag)
		if len(candidateParts) > 0 && len(currentParts) > 0 {
			currentMajor, _ := strconv.Atoi(currentParts[0])
			candidateMajor, _ := strconv.Atoi(candidateParts[0])
			if candidateMajor > 100 || (currentMajor < 100 && candidateMajor > currentMajor*10) {
				continue // Likely a date-based tag like 20260127
			}
		}
		
		// Check if newer
		if v.GreaterThan(cv) && (best == nil || v.GreaterThan(best)) {
			best = v
			bestTag = tag
		}
	}
	
	if bestTag != "" {
		return image + ":" + bestTag, nil
	}
	
	return "", nil
}
// ============================================================================
// PHASE 1: ALERTS & MONITORING
// ============================================================================

// Monitor resources and send alerts
func monitorResources() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if notifyChatID == 0 {
			continue
		}
		
		ctx := context.Background()
		containers, _ := cli.ContainerList(ctx, container.ListOptions{})
		
		for _, c := range containers {
			name := strings.TrimPrefix(c.Names[0], "/")
			alert, exists := resourceAlerts[name]
			if !exists || !alert.Enabled {
				continue
			}
			
			stats, err := cli.ContainerStats(ctx, c.ID, false)
			if err != nil {
				continue
			}
			defer stats.Body.Close()
			
			var v container.StatsResponse
			json.NewDecoder(stats.Body).Decode(&v)
			
			// CPU usage
			cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
			cpuPercent := (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
			
			// RAM usage
			ramPercent := float64(v.MemoryStats.Usage) / float64(v.MemoryStats.Limit) * 100.0
			
			if cpuPercent > alert.CPUThreshold {
				msg := fmt.Sprintf("⚠️ *Alerta de CPU*\n\n"+
					"Contenedor: `%s`\n"+
					"CPU: `%.1f%%` (umbral: %.0f%%)", name, cpuPercent, alert.CPUThreshold)
				bot.Send(tgbotapi.NewMessage(notifyChatID, msg))
			}
			
			if ramPercent > alert.RAMThreshold {
				msg := fmt.Sprintf("⚠️ *Alerta de RAM*\n\n"+
					"Contenedor: `%s`\n"+
					"RAM: `%.1f%%` (umbral: %.0f%%)", name, ramPercent, alert.RAMThreshold)
				m := tgbotapi.NewMessage(notifyChatID, msg)
				m.ParseMode = "Markdown"
				bot.Send(m)
			}
		}
	}
}

// Run health checks
func runHealthChecks() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		if notifyChatID == 0 {
			continue
		}
		
		for name, check := range healthChecks {
			if !check.Enabled {
				continue
			}
			
			var healthy bool
			if check.Type == "http" {
				resp, err := http.Get(check.Target)
				healthy = err == nil && resp.StatusCode == 200
				if resp != nil {
					resp.Body.Close()
				}
			} else if check.Type == "tcp" {
				conn, err := net.DialTimeout("tcp", check.Target, 5*time.Second)
				healthy = err == nil
				if conn != nil {
					conn.Close()
				}
			}
			
			if !healthy {
				msg := fmt.Sprintf("❌ *Health Check Failed*\n\n"+
					"Container: `%s`\n"+
					"Type: `%s`\n"+
					"Target: `%s`", name, check.Type, check.Target)
				m := tgbotapi.NewMessage(notifyChatID, msg)
				m.ParseMode = "Markdown"
				bot.Send(m)
			}
		}
	}
}

// Send scheduled reports
func sendScheduledReports() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		if notifyChatID == 0 || reportSchedule == "disabled" {
			continue
		}
		
		now := time.Now()
		shouldSend := false
		
		if reportSchedule == "daily" && now.Sub(lastReportTime) >= 24*time.Hour {
			shouldSend = true
		} else if reportSchedule == "weekly" && now.Sub(lastReportTime) >= 7*24*time.Hour {
			shouldSend = true
		}
		
		if !shouldSend {
			continue
		}
		
		ctx := context.Background()
		containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
		
		running := 0
		stopped := 0
		paused := 0
		
		for _, c := range containers {
			switch c.State {
			case "running":
				running++
			case "exited":
				stopped++
			case "paused":
				paused++
			}
		}
		
		msg := fmt.Sprintf("📊 *Reporte del Sistema*\n\n"+
			"🟢 Corriendo: %d\n"+
			"🔴 Detenidos: %d\n"+
			"🟡 Pausados: %d\n"+
			"📦 Total: %d\n\n"+
			"Fecha: %s", running, stopped, paused, len(containers), now.Format("2006-01-02 15:04"))
		
		m := tgbotapi.NewMessage(notifyChatID, msg)
		m.ParseMode = "Markdown"
		bot.Send(m)
		
		lastReportTime = now
		saveConfig()
	}
}

// Handle /alerts command
func handleAlerts(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{})
	
	text := "⚠️ *Uso de Recursos*\n\n"
	hasRunning := false
	
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		hasRunning = true
		name := strings.TrimPrefix(c.Names[0], "/")
		
		statsResp, err := cli.ContainerStats(ctx, c.ID, false)
		if err != nil {
			text += fmt.Sprintf("❌ `%s` - Error obteniendo stats\n\n", name)
			continue
		}
		
		var v container.StatsResponse
		if err := json.NewDecoder(statsResp.Body).Decode(&v); err != nil {
			statsResp.Body.Close()
			continue
		}
		statsResp.Body.Close()
		
		// CPU calculation using OnlineCPUs
		cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
		
		numCPU := v.CPUStats.OnlineCPUs
		if numCPU == 0 {
			numCPU = uint32(len(v.CPUStats.CPUUsage.PercpuUsage))
		}
		
		cpuPercent := 0.0
		if systemDelta > 0 && numCPU > 0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(numCPU) * 100.0
		}
		
		// Memory calculation
		memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 / 1024
		memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 / 1024
		memPercent := 0.0
		if memLimit > 0 {
			memPercent = (memUsage / memLimit) * 100
		}
		
		icon := "🟢"
		if cpuPercent > 80 || memPercent > 80 {
			icon = "🔴"
		} else if cpuPercent > 50 || memPercent > 50 {
			icon = "🟡"
		}
		
		text += fmt.Sprintf("%s `%s`\n   CPU: %.1f%% | RAM: %.2fGB (%.1f%%)\n\n", 
			icon, name, cpuPercent, memUsage, memPercent)
	}
	
	if !hasRunning {
		text += "Sin contenedores corriendo"
	}
	
	sendMessageWithClose(chatID, text)
}

// Handle /healthchecks command
func handleHealthChecks(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	
	text := "🏥 *Estado de Contenedores*\n\n"
	text += "Verificación del estado de salud de todos los contenedores:\n\n"
	
	running := 0
	stopped := 0
	unhealthy := 0
	
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		icon := "🟢"
		status := "Corriendo"
		
		if c.State == "exited" {
			icon = "🔴"
			status = "Detenido"
			stopped++
		} else if c.State == "running" {
			running++
			// Check if container has health status
			inspect, err := cli.ContainerInspect(ctx, c.ID)
			if err == nil && inspect.State.Health != nil {
				if inspect.State.Health.Status == "unhealthy" {
					icon = "🟡"
					status = "No saludable"
					unhealthy++
				}
			}
		}
		
		text += fmt.Sprintf("%s `%s` - %s\n", icon, name, status)
	}
	
	text += fmt.Sprintf("\n📊 *Resumen:*\n🟢 Corriendo: %d\n🔴 Detenidos: %d\n🟡 No saludables: %d", 
		running, stopped, unhealthy)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

// Handle /reports command
func handleReports(chatID int64) {
	text := fmt.Sprintf("📊 *Reportes Programados*\n\n"+
		"Configuración actual: `%s`\n\n"+
		"Selecciona la frecuencia de reportes:", reportSchedule)
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Diario", "report_daily"),
			tgbotapi.NewInlineKeyboardButtonData("📆 Semanal", "report_weekly"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔕 Desactivar", "report_disabled"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📤 Enviar ahora", "report_now"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// ============================================================================
// PHASE 3: SECURITY & AUDIT
// ============================================================================

// Add audit entry
func addAudit(userID int64, command, target string, success bool) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		UserID:    userID,
		Command:   command,
		Target:    target,
		Success:   success,
	}
	auditLog = append(auditLog, entry)
	
	// Keep only last 1000 entries
	if len(auditLog) > 1000 {
		auditLog = auditLog[len(auditLog)-1000:]
	}
	
	saveConfig()
}

// Handle /audit command
func handleAudit(chatID int64) {
	text := "📋 *Registro de Auditoría*\n\n"
	
	if len(auditLog) == 0 {
		text += "Sin entradas de auditoría"
	} else {
		// Show last 10 entries
		start := len(auditLog) - 10
		if start < 0 {
			start = 0
		}
		
		loc, _ := time.LoadLocation("America/Bogota")
		
		for i := len(auditLog) - 1; i >= start; i-- {
			entry := auditLog[i]
			status := "✅"
			if !entry.Success {
				status = "❌"
			}
			localTime := entry.Timestamp.In(loc)
			text += fmt.Sprintf("%s `%s` - %s\n  %s %s\n\n", 
				status, entry.Command, entry.Target, 
				localTime.Format("2006-01-02"), localTime.Format("15:04"))
		}
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📥 Exportar", "audit_export"),
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Limpiar", "audit_clear"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Scan image with Trivy
func scanImage(imageName string) (string, error) {
	cmd := exec.Command("trivy", "image", "--severity", "HIGH,CRITICAL", "--format", "json", imageName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	var result struct {
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID string `json:"VulnerabilityID"`
				Severity        string `json:"Severity"`
				Title           string `json:"Title"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	
	if err := json.Unmarshal(output, &result); err != nil {
		return "", err
	}
	
	critical := 0
	high := 0
	
	for _, r := range result.Results {
		for _, v := range r.Vulnerabilities {
			if v.Severity == "CRITICAL" {
				critical++
			} else if v.Severity == "HIGH" {
				high++
			}
		}
	}
	
	return fmt.Sprintf("🔒 *Escaneo de Seguridad*\n\n"+
		"Imagen: `%s`\n\n"+
		"🔴 Críticas: %d\n"+
		"🟠 Altas: %d", imageName, critical, high), nil
}

// Handle /scan command
func handleScan(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	
	text := "🔒 *Escanear Vulnerabilidades*\n\nSelecciona un contenedor:"
	
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 "+name, "scan:"+name),
		))
	}
	
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// Handle /webhooks command
func handleWebhooks(chatID int64) {
	text := "🔗 *Webhooks*\n\n"
	
	if len(webhooks) == 0 {
		text += "📋 Sin webhooks configurados\n\n"
		text += "Los webhooks envían notificaciones HTTP cuando ocurren eventos.\n\n"
		text += "*Eventos disponibles:*\n"
		text += "• container.start\n"
		text += "• container.stop\n"
		text += "• container.die\n"
		text += "• image.update"
	} else {
		text += "✅ *Webhooks configurados:*\n\n"
		for name, wh := range webhooks {
			status := "❌"
			if wh.Enabled {
				status = "✅"
			}
			text += fmt.Sprintf("%s `%s`\n   URL: %s\n   Eventos: %d\n\n", status, name, wh.URL, len(wh.Events))
		}
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Agregar Webhook", "webhook_add"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📖 Ver Configuración Manual", "webhook_manual"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Send webhook notification
func sendWebhook(event, target string) {
	for _, wh := range webhooks {
		if !wh.Enabled {
			continue
		}
		
		found := false
		for _, e := range wh.Events {
			if e == event || e == "all" {
				found = true
				break
			}
		}
		
		if !found {
			continue
		}
		
		payload := map[string]string{
			"event":     event,
			"target":    target,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", wh.URL, strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		
		for k, v := range wh.Headers {
			req.Header.Set(k, v)
		}
		
		client := &http.Client{Timeout: 10 * time.Second}
		client.Do(req)
	}
}

// Handle /policies command
func handlePolicies(chatID int64) {
	text := "⚙️ *Políticas de Actualización*\n\n"
	text += "Las políticas de actualización permiten automatizar las actualizaciones de contenedores.\n\n"
	text += "📋 *Configuración actual:*\n"
	
	if len(autoUpdateContainers) == 0 {
		text += "Sin políticas configuradas\n\n"
	} else {
		for name, enabled := range autoUpdateContainers {
			status := "❌"
			if enabled {
				status = "✅"
			}
			text += fmt.Sprintf("%s `%s`\n", status, name)
		}
		text += "\n"
	}
	
	text += "💡 _Usa /autoupdate para configurar actualizaciones automáticas por contenedor_"
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

// ============================================================================
// PHASE 4: NETWORKING & REGISTRY
// ============================================================================


// ============================================================================
// PHASE 4: NETWORKING & REGISTRY HANDLERS
// ============================================================================

// Handle /registries command
func handleRegistries(chatID int64) {
	text := "📦 *Registries Privados*\n\n"
	
	text += "*Registries soportados:*\n"
	text += "• Docker Hub (docker.io)\n"
	text += "• GitHub (ghcr.io)\n"
	text += "• GitLab (registry.gitlab.com)\n"
	text += "• Registries privados\n\n"
	
	text += "*Cómo autenticar:*\n"
	text += "```bash\n"
	text += "docker login ghcr.io\n"
	text += "docker login registry.example.com\n"
	text += "```\n\n"
	
	text += "💡 _Las credenciales se guardan en el host_"
	
	sendMessageWithClose(chatID, text)
}

// Handle /cleanup command
func handleCleanup(chatID int64) {
	ctx := context.Background()
	
	// Get all images
	images, _ := cli.ImageList(ctx, image.ListOptions{All: true})
	
	// Get all containers
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	
	// Find used images
	usedImages := make(map[string]bool)
	for _, c := range containers {
		usedImages[c.ImageID] = true
	}
	
	// Find orphaned images
	orphaned := []string{}
	var orphanedSize int64
	
	for _, img := range images {
		if !usedImages[img.ID] && len(img.RepoTags) > 0 {
			orphaned = append(orphaned, img.RepoTags[0])
			orphanedSize += img.Size
		}
	}
	
	sizeMB := float64(orphanedSize) / 1024 / 1024
	sizeText := fmt.Sprintf("%.1f MB", sizeMB)
	if sizeMB > 1024 {
		sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
	}
	
	text := fmt.Sprintf("🧹 *Limpieza Inteligente*\n\n"+
		"Imágenes huérfanas: %d\n"+
		"Espacio a liberar: %s\n\n", len(orphaned), sizeText)
	
	if len(orphaned) > 0 {
		text += "Imágenes detectadas:\n"
		for i, img := range orphaned {
			if i >= 10 {
				text += fmt.Sprintf("... y %d más\n", len(orphaned)-10)
				break
			}
			text += fmt.Sprintf("• `%s`\n", img)
		}
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Limpiar todo", "cleanup_all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Handle /ports command
func handlePorts(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{})
	
	text := "🔌 *Puertos Expuestos*\n\n"
	
	seen := make(map[string]bool)
	
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		for _, port := range c.Ports {
			if port.PublicPort > 0 {
				key := fmt.Sprintf("%d-%s-%s", port.PublicPort, name, port.Type)
				if !seen[key] {
					seen[key] = true
					text += fmt.Sprintf("• `%d` → `%s` (%s)\n", port.PublicPort, name, port.Type)
				}
			}
		}
	}
	
	if len(seen) == 0 {
		text += "Sin puertos expuestos"
	}
	
	sendMessageWithClose(chatID, text)
}
