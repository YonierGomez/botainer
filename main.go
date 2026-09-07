package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	semver "github.com/Masterminds/semver/v3"
	"github.com/YonierGomez/botainer/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	botVersion     = "2.6.0"                      // v2.6.0: full i18n — all user-facing strings now support es/en via getText()
	newsChannelURL = "https://t.me/botainer_news" // Canal de novedades
	configFile     = "/data/config.json"          // Persistence file
)

var (
	bot                  *tgbotapi.BotAPI
	cli                  *client.Client
	notifyChatID         int64
	allowedUsers         []int64
	favorites            = make(map[int64][]string)
	commandHistory       = make(map[int64][]string)
	userState            = make(map[int64]string)
	createData           = make(map[int64]map[string]string)
	autoUpdateContainers = make(map[string]bool)
	trackedImages        = make(map[string]string)          // image:tag -> last known digest
	trackedCharts        = make(map[string]ChartInfo)       // repo/chart -> chart info
	rollbackHistory      = make(map[string][]RollbackEntry) // container -> history (max 5)
	templates            = make(map[string]ContainerTemplate)
	maintenanceMode      bool
	maintenancePaused    []string           // containers paused by maintenance mode
	updateTransaction    *UpdateTransaction // track ongoing update for recovery

	// Phase 1: Alerts & Monitoring
	resourceAlerts = make(map[string]ResourceAlert) // container -> alert config
	healthChecks   = make(map[string]HealthCheck)   // container -> health check config
	reportSchedule = "daily"                        // daily, weekly, or disabled
	lastReportTime time.Time

	// Phase 3: Security & Audit
	auditLog       []AuditEntry
	webhooks       = make(map[string]Webhook)      // name -> webhook config
	updatePolicies = make(map[string]UpdatePolicy) // container -> policy

	// Phase 4: Networking & Registry
	registries           = make(map[string]Registry)         // name -> registry config
	criticalContainers   = map[string]bool{"botainer": true} // containers to never pause
	checkUpdatesInterval = 6 * time.Hour
	enableAutoCheck      = true
	enableStartupNotif   = true
	configMutex          sync.Mutex
	stateMutex           sync.Mutex
	language             = "es" // Default language
	translations         = make(map[string]string)
	containerIcons       = map[string]string{
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
	RollbackHistory      map[string][]RollbackEntry   `json:"rollbackHistory"` // container -> history
	Templates            map[string]ContainerTemplate `json:"templates"`
	MaintenanceMode      bool                         `json:"maintenanceMode"`
	MaintenancePaused    []string                     `json:"maintenancePaused"` // containers paused by maintenance
	UpdateInProgress     *UpdateTransaction           `json:"updateInProgress"`  // track ongoing updates

	// Phase 1
	ResourceAlerts map[string]ResourceAlert `json:"resourceAlerts"`
	HealthChecks   map[string]HealthCheck   `json:"healthChecks"`
	ReportSchedule string                   `json:"reportSchedule"`
	LastReportTime time.Time                `json:"lastReportTime"`

	// Phase 3
	AuditLog       []AuditEntry            `json:"auditLog"`
	Webhooks       map[string]Webhook      `json:"webhooks"`
	UpdatePolicies map[string]UpdatePolicy `json:"updatePolicies"`

	// Phase 4
	Registries map[string]Registry `json:"registries"`
}

// UpdateTransaction tracks an update operation for recovery
type UpdateTransaction struct {
	ID           string            `json:"id"`
	StartTime    time.Time         `json:"startTime"`
	ChatID       int64             `json:"chatId"`
	Containers   []ContainerUpdate `json:"containers"`
	CompletedIdx int               `json:"completedIdx"` // index of last completed update
	Status       string            `json:"status"`       // "in_progress", "completed", "failed"
}

type ContainerUpdate struct {
	Name        string `json:"name"`
	Service     string `json:"service,omitempty"` // Docker Compose service name (may differ from container name)
	Project     string `json:"project"`
	OldImage    string `json:"oldImage"`
	NewImage    string `json:"newImage"`
	ComposeFile string `json:"composeFile,omitempty"`
	Status      string `json:"status"` // "pending", "success", "failed"
	Error       string `json:"error,omitempty"`
}

// RollbackEntry stores a previous image for a container
type RollbackEntry struct {
	Image     string    `json:"image"`
	ImageID   string    `json:"imageId"`
	Timestamp time.Time `json:"timestamp"`
}

// ContainerTemplate stores a container configuration for reuse
type ContainerTemplate struct {
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Cmd           []string          `json:"cmd,omitempty"`
	Env           []string          `json:"env,omitempty"`
	Ports         map[string]string `json:"ports,omitempty"` // hostPort -> containerPort
	Volumes       []string          `json:"volumes,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	NetworkMode   string            `json:"networkMode,omitempty"`
	RestartPolicy string            `json:"restartPolicy,omitempty"`
	IsPublic      bool              `json:"isPublic"`
	CreatedBy     int64             `json:"createdBy"`
	CreatedAt     time.Time         `json:"createdAt"`
	UsageCount    int               `json:"usageCount"`
}

type ImageHistory struct {
	Image     string    `json:"image"`
	Digest    string    `json:"digest"`
	Timestamp time.Time `json:"timestamp"`
}

type RegistryAuth struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
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
	Schedule    string `json:"schedule"`    // cron format or "immediate"
	MinFreeRAM  int    `json:"minFreeRam"`  // MB
	MinFreeDisk int    `json:"minFreeDisk"` // GB
	AutoApprove bool   `json:"autoApprove"` // auto-update without confirmation
	Enabled     bool   `json:"enabled"`
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
	Version    string `json:"version"`
	AppVersion string `json:"app_version"`
	Repository struct {
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

// writeConfigLocked writes config to disk. Caller MUST hold configMutex.
func writeConfigLocked() {
	cfg := Config{
		AutoUpdateContainers: autoUpdateContainers,
		TrackedImages:        trackedImages,
		TrackedCharts:        trackedCharts,
		LastCheck:            time.Now(),
		RollbackHistory:      rollbackHistory,
		Templates:            templates,
		MaintenanceMode:      maintenanceMode,
		MaintenancePaused:    maintenancePaused,
		UpdateInProgress:     updateTransaction,
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

	os.MkdirAll("/data", 0755)

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		log.Printf("Error saving config: %v", err)
	}
}

// Save configuration to file
func saveConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()
	writeConfigLocked()
}

// Backup compose file before modification
func backupComposeFile(composeFile string) (string, error) {
	backupDir := "/data/backups"
	os.MkdirAll(backupDir, 0755)

	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s/%s.%s.bak", backupDir, filepath.Base(composeFile), timestamp)

	data, err := os.ReadFile(composeFile)
	if err != nil {
		return "", fmt.Errorf("failed to read compose file: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	log.Printf("[backup] Created backup: %s", backupPath)
	return backupPath, nil
}

// Restore compose file from backup
func restoreComposeFile(composeFile, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(composeFile, data, 0644); err != nil {
		return fmt.Errorf("failed to restore compose file: %w", err)
	}

	log.Printf("[backup] Restored from backup: %s", backupPath)
	return nil
}

// Validate compose file syntax
func validateComposeFile(composeFile string) error {
	output, err := runComposeCmd(10*time.Second, composeFile, "config", "--quiet")
	if err != nil {
		return fmt.Errorf("invalid compose file: %s", output)
	}
	return nil
}

// Retry function with exponential backoff
func retryWithBackoff(operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			delay := baseDelay * time.Duration(1<<uint(i-1)) // exponential: 5s, 10s, 20s
			log.Printf("[retry] Attempt %d/%d failed, waiting %v before retry", i, maxRetries, delay)
			time.Sleep(delay)
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		log.Printf("[retry] Attempt %d/%d error: %v", i+1, maxRetries, lastErr)
	}
	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// Transaction management functions

// startUpdateTransaction creates a new update transaction
func startUpdateTransaction(chatID int64, containers []ContainerUpdate) {
	configMutex.Lock()
	defer configMutex.Unlock()

	updateTransaction = &UpdateTransaction{
		ID:           fmt.Sprintf("upd_%d_%d", chatID, time.Now().Unix()),
		StartTime:    time.Now(),
		ChatID:       chatID,
		Containers:   containers,
		CompletedIdx: -1,
		Status:       "in_progress",
	}

	log.Printf("[transaction] Started: %s with %d containers", updateTransaction.ID, len(containers))
	writeConfigLocked()
}

// updateTransactionProgress updates the progress of current container
func updateTransactionProgress(idx int, status, errorMsg string) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if updateTransaction == nil || idx >= len(updateTransaction.Containers) {
		return
	}

	updateTransaction.Containers[idx].Status = status
	updateTransaction.Containers[idx].Error = errorMsg
	if status == "success" {
		updateTransaction.CompletedIdx = idx
	}

	log.Printf("[transaction] Container %d/%d: %s - %s",
		idx+1, len(updateTransaction.Containers),
		updateTransaction.Containers[idx].Name, status)
	writeConfigLocked()
}

// completeUpdateTransaction marks transaction as completed
func completeUpdateTransaction(status string) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if updateTransaction == nil {
		return
	}

	log.Printf("[transaction] Completed: %s with status: %s", updateTransaction.ID, status)
	if status == "completed" {
		updateTransaction = nil
	} else {
		updateTransaction.Status = status
	}
	writeConfigLocked()
}

// clearUpdateTransaction removes the current transaction
func clearUpdateTransaction() {
	configMutex.Lock()
	defer configMutex.Unlock()

	if updateTransaction != nil {
		log.Printf("[transaction] Cleared: %s", updateTransaction.ID)
		updateTransaction = nil
		writeConfigLocked()
	}
}

// Pre-update validation functions

// validatePreUpdate performs all pre-update checks.
// serviceName is the Docker Compose service name; if empty, containerName is used as fallback.
func validatePreUpdate(containerName, project, composeFile, newImage string, serviceName ...string) error {
	// 1. Validate compose file if it's a compose project
	if project != "" && composeFile != "" {
		if err := validateComposeFile(composeFile); err != nil {
			return fmt.Errorf("compose file validation failed: %w", err)
		}

		// 2. Validate service exists in compose (use service name if provided)
		svcName := containerName
		if len(serviceName) > 0 && serviceName[0] != "" {
			svcName = serviceName[0]
		}
		if !serviceExistsInCompose(composeFile, svcName) {
			return fmt.Errorf("service '%s' not found in compose file", svcName)
		}
	}

	// 3. Validate new image exists
	if newImage != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reader, err := cli.ImagePull(ctx, newImage, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull new image '%s': %w", newImage, err)
		}
		io.Copy(io.Discard, reader)
		reader.Close()
	}

	// 4. Check disk space (require at least 1GB free)
	if err := checkDiskSpace(1024); err != nil {
		return err
	}

	return nil
}

// checkDiskSpace verifies there's enough free disk space (in MB)
func checkDiskSpace(requiredMB int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/var/lib/docker", &stat); err != nil {
		// Fallback to root if docker dir not accessible
		if err := syscall.Statfs("/", &stat); err != nil {
			return fmt.Errorf("failed to check disk space: %w", err)
		}
	}

	availableMB := int64(stat.Bavail * uint64(stat.Bsize) / 1024 / 1024)
	if availableMB < requiredMB {
		return fmt.Errorf("insufficient disk space: %d MB available, %d MB required", availableMB, requiredMB)
	}

	log.Printf("[validation] Disk space OK: %d MB available", availableMB)
	return nil
}

// checkAndRecoverTransaction checks for incomplete transactions at startup
func checkAndRecoverTransaction() {
	configMutex.Lock()
	defer configMutex.Unlock()

	if updateTransaction == nil {
		return
	}

	// Found incomplete transaction
	log.Printf("[recovery] Found incomplete transaction: %s (status: %s)",
		updateTransaction.ID, updateTransaction.Status)

	if updateTransaction.Status == "completed" || updateTransaction.Status == "failed" {
		// Transaction finished but not cleared, just clear it
		log.Printf("[recovery] Transaction already finished, clearing")
		updateTransaction = nil
		writeConfigLocked()
		return
	}

	// Transaction was interrupted, notify user
	chatID := updateTransaction.ChatID
	completed := updateTransaction.CompletedIdx + 1
	total := len(updateTransaction.Containers)

	text := getText("recovery_transaction_detected",
		updateTransaction.ID,
		updateTransaction.StartTime.Format("2006-01-02 15:04:05"),
		completed, total)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_recovery_continue"), "recovery_continue"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_recovery_rollback"), "recovery_rollback"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_recovery_mark_complete"), "recovery_complete"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "recovery_cancel"),
		),
	)

	if _, err := bot.Send(msg); err != nil {
		log.Printf("[recovery] Failed to send recovery message: %v", err)
	}
}

// validatePostUpdate performs post-update validation
func validatePostUpdate(containerName string) error {
	ctx := context.Background()

	// Wait 10 seconds for container to stabilize
	log.Printf("[validation] Waiting 10 seconds for container to stabilize: %s", containerName)
	time.Sleep(10 * time.Second)

	// Check if container is running
	inspect, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if !inspect.State.Running {
		return fmt.Errorf("container is not running (status: %s)", inspect.State.Status)
	}

	// Check health status if health check is configured
	if inspect.State.Health != nil {
		log.Printf("[validation] Health status: %s", inspect.State.Health.Status)
		if inspect.State.Health.Status == "unhealthy" {
			return fmt.Errorf("container health check failed")
		}
		// If starting, wait a bit more
		if inspect.State.Health.Status == "starting" {
			log.Printf("[validation] Health check still starting, waiting 5 more seconds")
			time.Sleep(5 * time.Second)
			inspect, _ = cli.ContainerInspect(ctx, containerName)
			if inspect.State.Health != nil && inspect.State.Health.Status == "unhealthy" {
				return fmt.Errorf("container health check failed after waiting")
			}
		}
	}

	// Check logs for common error patterns
	logStr := readContainerLogs(ctx, cli, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "50",
	})
	if logStr != "" {
		errorPatterns := []string{"fatal error", "panic:"}
		for _, pattern := range errorPatterns {
			if strings.Contains(strings.ToLower(logStr), pattern) {
				log.Printf("[validation] Warning: Found fatal error patterns in logs")
				return fmt.Errorf("fatal error patterns found in container logs")
			}
		}
	}

	log.Printf("[validation] Post-update validation passed for: %s", containerName)
	return nil
}

// readContainerLogs reads Docker container logs handling both TTY and non-TTY containers.
// Non-TTY containers use Docker's multiplexed stream format (8-byte headers per frame);
// TTY containers use a raw stream. Returns sanitized UTF-8 text safe for Telegram/JSON.
func readContainerLogs(ctx context.Context, cli *client.Client, containerID string, opts container.LogsOptions) string {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	isTTY := err == nil && inspect.Config.Tty

	reader, err := cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return ""
	}
	defer reader.Close()

	var content string
	if isTTY {
		data, _ := io.ReadAll(reader)
		content = string(data)
	} else {
		var stdout, stderr bytes.Buffer
		stdcopy.StdCopy(&stdout, &stderr, reader)
		content = stdout.String() + stderr.String()
	}

	// Sanitize: replace invalid UTF-8 and drop non-printable control chars (keep \n \t \r)
	content = strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return '?'
		}
		if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(content, "?"))

	return content
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
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	return keyboard
}

// truncateError safely truncates error messages to maxLen characters
// Always use this instead of err.Error()[:n] to avoid slice bounds panics
func truncateError(err error, maxLen int) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen]
}

func sendMessageWithClose(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
	return runCmdInDirWithTimeout("", timeout, cmd, args...)
}

func runCmdInDirWithTimeout(dir string, timeout time.Duration, cmd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(ctx, cmd, args...)
	if dir != "" {
		command.Dir = dir
	}
	out, err := command.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("comando excedió timeout de %v", timeout)
	}

	return string(out), err
}

// runComposeCmd runs a docker compose command with the working directory set to
// the directory containing composeFile. This ensures $PWD-based volume paths resolve correctly.
func runComposeCmd(timeout time.Duration, composeFile string, args ...string) (string, error) {
	dir := filepath.Dir(composeFile)
	cmdArgs := append([]string{"compose", "-f", composeFile}, args...)
	return runCmdInDirWithTimeout(dir, timeout, "docker", cmdArgs...)
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

// resolveComposeFile resolves workDir and composeFile for a given compose project name.
// Returns an error if either cannot be found.
func resolveComposeFile(project string) (workDir, composeFile string, err error) {
	workDir = getComposeWorkDir(project)
	if workDir == "" {
		return "", "", fmt.Errorf("workdir not found for project %s", project)
	}
	composeFile = findComposeFile(workDir)
	if composeFile == "" {
		return workDir, "", fmt.Errorf("compose file not found in %s", workDir)
	}
	return workDir, composeFile, nil
}

// serviceExistsInCompose checks if a service name exists in the compose file
func serviceExistsInCompose(composeFile, serviceName string) bool {
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return false
	}

	// Simple check: look for "serviceName:" in the services section
	lines := strings.Split(string(data), "\n")
	inServices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if inServices && strings.HasPrefix(line, "  ") && strings.HasSuffix(trimmed, ":") {
			// This is a service definition
			service := strings.TrimSuffix(trimmed, ":")
			if service == serviceName {
				return true
			}
		}
		// Exit services section if we hit another top-level key
		if inServices && !strings.HasPrefix(line, " ") && trimmed != "" && strings.HasSuffix(trimmed, ":") {
			break
		}
	}
	return false
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

			name := containerFirstName(cont)
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
	msg := tgbotapi.NewMessage(chatID, getText("bot_version_header", botVersion))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(getText("button_news_channel"), newsChannelURL),
			tgbotapi.NewInlineKeyboardButtonURL("⭐ GitHub", "https://github.com/YonierGomez/botainer"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	bot.Send(msg)
}

func handleStart(chatID int64) {
	// Check bot version and show update notification if available
	checkBotVersion(chatID)

	// Get Mini App URL from environment or use default
	miniAppURL := os.Getenv("MINI_APP_URL")
	if miniAppURL == "" {
		miniAppURL = "http://localhost:8080" // Default for local testing
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_list"), "cmd:list"),
			tgbotapi.NewInlineKeyboardButtonData("📊 PS", "cmd:ps"),
			tgbotapi.NewInlineKeyboardButtonData("🖥️ Stats", "cmd:stats"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📁 Compose", "cmd:compose"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "cmd:inspect_menu"),
			tgbotapi.NewInlineKeyboardButtonData("⚙️ Exec", "cmd:exec_menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🖼️ Images", "cmd:images"),
			tgbotapi.NewInlineKeyboardButtonData("💾 Volumes", "cmd:volumes"),
			tgbotapi.NewInlineKeyboardButtonData("🌐 Networks", "cmd:networks"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_check_updates"), "cmd:check_updates"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Prune", "cmd:prune_menu"),
			tgbotapi.NewInlineKeyboardButtonData("🔧 Diagnose", "cmd:diagnose"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, getText("start_welcome"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleNetworks(chatID int64) {
	ctx := context.Background()
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	for _, net := range networks {
		containers := []string{}
		for _, ep := range net.Containers {
			containers = append(containers, ep.Name)
		}

		project := net.Labels["com.docker.compose.project"]

		text := getText("network_info_header", net.Name, net.Driver, net.Scope)
		if len(containers) > 0 {
			text += getText("network_containers_line", strings.Join(containers, ", "))
		}
		if project != "" {
			text += getText("project_line", project)
		} else {
			text += getText("no_containers_line")
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect_net:"+net.Name),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_delete"), "rmnet_confirm:"+net.Name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
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

		text := getText("image_info_header", tag, img.ID[:19], sizeText)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect_img:"+img.ID),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_delete"), "rmi_confirm:"+img.ID),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
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
			containerNames = append(containerNames, containerFirstName(c))
		}

		project := vol.Labels["com.docker.compose.project"]

		var text string
		if len(containerNames) > 0 {
			text = getText("volume_used_by", vol.Name, strings.Join(containerNames, ", "))
			if project != "" {
				text += getText("project_line", project)
			}
		} else if project != "" {
			text = getText("volume_project_only", vol.Name, project)
		} else {
			text = getText("volume_unused", vol.Name)
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect_vol:"+vol.Name),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_delete"), "rmvol_confirm:"+vol.Name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💾 Backup", "backup:"+vol.Name),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handlePS(chatID int64) {
	loadingID := sendLoading(chatID, getText("loading_stats"))
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_running_containers_ps"))
		return
	}

	type result struct {
		name, icon, status, image, project, cpu, mem string
	}

	results := make(chan result, len(containers))

	for _, c := range containers {
		go func(c types.Container) {
			name := containerFirstName(c)
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

		text := getText("ps_container_header", r.icon, r.name, r.status, r.image)
		if r.project != "" {
			text += getText("project_line_mid", r.project)
		}
		text += getText("ps_cpu_ram_line", r.cpu, r.mem)

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+r.name),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_restart"), "restart:"+r.name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_stop"), "stop:"+r.name),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect:"+r.name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
		bot.Send(msg)
	}
}

func handleRunning(chatID int64) {
	loadingID := sendLoading(chatID, getText("loading_containers"))
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	for _, c := range containers {
		name := containerFirstName(c)
		icon := getIcon(name)
		statusIcon := "🔴"
		if c.State == "running" {
			statusIcon = "🟢"
		} else if c.State == "paused" {
			statusIcon = "🟡"
		}

		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		project := inspect.Config.Labels["com.docker.compose.project"]

		text := getText("running_container_header", statusIcon, icon, name, c.Status, c.Image)
		if project != "" {
			text = getText("running_container_header_project", statusIcon, icon, name, c.Status, c.Image, project)
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		var keyboard tgbotapi.InlineKeyboardMarkup
		if c.State == "running" {
			keyboard = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_restart"), "restart:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_stop"), "stop:"+name),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
				),
			)
		} else {
			keyboard = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_start"), "start:"+name),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove"), "remove:"+name),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
				),
			)
		}
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handleList(chatID int64) {
	loadingID := sendLoading(chatID, getText("loading_list_containers"))
	defer deleteMsg(chatID, loadingID)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_containers"))
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
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
			name2 := containerFirstName(containers[i+1])
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("containers_count_header", len(containers)))
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
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_containers"))
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
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
			name2 := containerFirstName(containers[i+1])
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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

	// Recovery callbacks
	if strings.HasPrefix(query.Data, "recovery_") {
		bot.Send(tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))

		switch query.Data {
		case "recovery_continue":
			sendMessageWithClose(chatID, getText("recovery_continue_soon"))
			// TODO: Implement continue from last checkpoint

		case "recovery_rollback":
			sendMessageWithClose(chatID, getText("recovery_rollback_soon"))
			// TODO: Implement rollback of completed updates

		case "recovery_complete":
			completeUpdateTransaction("completed")
			sendMessageWithClose(chatID, getText("recovery_marked_complete"))

		case "recovery_cancel":
			clearUpdateTransaction()
			sendMessageWithClose(chatID, getText("recovery_cancelled"))
		}
		return
	}

	if query.Data == "updateall_confirm" {
		log.Printf("[updateall] Callback received from chatID: %d", chatID)
		bot.Send(tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID))

		go func() {
			stateMutex.Lock()
			updatesJSON := ""
			if data := createData[chatID]; data != nil {
				updatesJSON = data["pending_updates"]
			}
			log.Printf("[updateall] Retrieved updates JSON length: %d", len(updatesJSON))
			delete(createData, chatID)
			stateMutex.Unlock()

			if updatesJSON == "" {
				log.Printf("[updateall] ERROR: No pending updates found")
				sendMessageWithClose(chatID, getText("no_pending_updates"))
				return
			}

			type updateInfo struct {
				ImageTag   string `json:"imageTag"`
				Containers []struct {
					Name    string `json:"name"`
					Service string `json:"service,omitempty"`
					Project string `json:"project"`
				} `json:"containers"`
				OldID string `json:"oldID"`
				NewID string `json:"newID"`
				Size  int64  `json:"size"`
			}

			var updates []updateInfo
			if err := json.Unmarshal([]byte(updatesJSON), &updates); err != nil {
				sendMessageWithClose(chatID, getText("generic_error", err.Error()))
				return
			}

			// Build transaction container list
			var transactionContainers []ContainerUpdate
			for _, upd := range updates {
				for _, c := range upd.Containers {
					svc := c.Service
					if svc == "" {
						svc = c.Name // fallback
					}
					transactionContainers = append(transactionContainers, ContainerUpdate{
						Name:     c.Name,
						Service:  svc,
						Project:  c.Project,
						OldImage: upd.ImageTag,
						NewImage: upd.ImageTag,
						Status:   "pending",
					})
				}
			}

			// Start transaction
			startUpdateTransaction(chatID, transactionContainers)

			totalContainers := len(transactionContainers)

			// Send progress message
			progressMsg := tgbotapi.NewMessage(chatID, getText("updating_n_containers", totalContainers))
			progressMsg.ParseMode = "Markdown"
			sentMsg, _ := bot.Send(progressMsg)

			// Process updates
			successes := []string{}
			failures := []string{}
			rollbacks := []string{}

			for idx, containerUpd := range transactionContainers {
				containerName := containerUpd.Name
				project := containerUpd.Project
				// Use service name for docker compose commands; fall back to container name
				serviceName := containerUpd.Service
				if serviceName == "" {
					serviceName = containerName
				}

				// Update progress
				edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("updating_progress", idx+1, totalContainers, containerName))
				edit.ParseMode = "Markdown"
				bot.Send(edit)

				log.Printf("[updateall] Processing %d/%d: %s (service: %s, project: %s)", idx+1, totalContainers, containerName, serviceName, project)

				var composeFile string
				var backupPath string

				// Pre-validation
				if project != "" {
					_, cf, resolveErr := resolveComposeFile(project)
					if resolveErr != nil {
						log.Printf("[updateall] ERROR: %v", resolveErr)
						updateTransactionProgress(idx, "failed", resolveErr.Error())
						failures = append(failures, fmt.Sprintf("%s (%v)", containerName, resolveErr))
						continue
					}
					composeFile = cf

					// Store compose file in transaction
					configMutex.Lock()
					if updateTransaction != nil && idx < len(updateTransaction.Containers) {
						updateTransaction.Containers[idx].ComposeFile = composeFile
						writeConfigLocked()
					}
					configMutex.Unlock()

					// Pre-update validation (pass service name for compose check)
					if err := validatePreUpdate(containerName, project, composeFile, "", serviceName); err != nil {
						log.Printf("[updateall] Pre-validation failed: %v", err)
						updateTransactionProgress(idx, "failed", err.Error())
						failures = append(failures, fmt.Sprintf("%s (validation: %s)", containerName, truncateError(err, 40)))
						continue
					}

					// Backup compose file
					var err error
					backupPath, err = backupComposeFile(composeFile)
					if err != nil {
						log.Printf("[updateall] Backup failed: %v", err)
						updateTransactionProgress(idx, "failed", "backup failed")
						failures = append(failures, fmt.Sprintf("%s (backup failed)", containerName))
						continue
					}
					log.Printf("[updateall] Backup created: %s", backupPath)
				}

				// Perform update with retry
				updateErr := retryWithBackoff(func() error {
					if project != "" {
						// Compose update - pull first, then recreate (mirrors: docker compose pull && docker compose up -d)
						log.Printf("[updateall] Running: docker compose -f %s pull %s", composeFile, serviceName)
						pullOut, pullErr := runComposeCmd(5*time.Minute, composeFile, "pull", serviceName)
						if pullErr != nil {
							return fmt.Errorf("compose pull failed: %s", pullOut)
						}
						log.Printf("[updateall] Running: docker compose -f %s up -d --no-deps %s", composeFile, serviceName)
						output, err := runComposeCmd(3*time.Minute, composeFile, "up", "-d", "--no-deps", serviceName)
						if err != nil {
							return fmt.Errorf("compose up failed: %s", output)
						}
						return nil
					} else {
						// Standalone update
						log.Printf("[updateall] Recreating standalone: %s", containerName)
						return recreateWithNewImage(containerName)
					}
				}, 3, 5*time.Second)

				if updateErr != nil {
					log.Printf("[updateall] Update failed after retries: %v", updateErr)
					updateTransactionProgress(idx, "failed", updateErr.Error())
					failures = append(failures, fmt.Sprintf("%s (%s)", containerName, truncateError(updateErr, 50)))

					// Rollback if backup exists
					if backupPath != "" {
						log.Printf("[updateall] Rolling back: %s", containerName)
						if err := restoreComposeFile(composeFile, backupPath); err != nil {
							log.Printf("[updateall] Rollback failed: %v", err)
						} else {
							runComposeCmd(2*time.Minute, composeFile, "up", "-d", serviceName)
							rollbacks = append(rollbacks, containerName)
						}
					}
					continue
				}

				// Post-update validation
				if err := validatePostUpdate(containerName); err != nil {
					log.Printf("[updateall] Post-validation failed: %v", err)
					updateTransactionProgress(idx, "failed", "post-validation failed: "+err.Error())
					failures = append(failures, fmt.Sprintf("%s (validation failed)", containerName))

					// Automatic rollback
					if backupPath != "" {
						log.Printf("[updateall] Auto-rollback due to validation failure: %s", containerName)
						if err := restoreComposeFile(composeFile, backupPath); err != nil {
							log.Printf("[updateall] Rollback failed: %v", err)
						} else {
							runComposeCmd(2*time.Minute, composeFile, "up", "-d", serviceName)
							rollbacks = append(rollbacks, containerName)
						}
					}
					continue
				}

				// Success
				log.Printf("[updateall] SUCCESS: %s", containerName)
				updateTransactionProgress(idx, "success", "")
				successes = append(successes, containerName)
			}

			// Complete transaction
			if len(failures) == 0 {
				completeUpdateTransaction("completed")
			} else {
				completeUpdateTransaction("failed")
			}

			// Delete progress
			bot.Send(tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID))

			// Verify status of updated containers
			running := []string{}
			stopped := []string{}
			for _, name := range successes {
				inspect, err := cli.ContainerInspect(ctx, name)
				if err == nil && inspect.State.Running {
					running = append(running, name)
				} else {
					stopped = append(stopped, name)
				}
			}

			// Final report
			text := getText("updateall_final_report", len(successes), len(failures))

			if len(rollbacks) > 0 {
				text += getText("updateall_rollbacks_line", len(rollbacks))
			}

			text += getText("updateall_total_line", totalContainers)

			if len(running) > 0 {
				text += getText("updateall_running_header")
				for _, name := range running {
					text += getText("updateall_running_item", getIcon(name), name)
				}
				text += "\n"
			}

			if len(stopped) > 0 {
				text += getText("updateall_stopped_header")
				for _, name := range stopped {
					text += getText("updateall_stopped_item", getIcon(name), name)
				}
				text += "\n"
			}

			if len(rollbacks) > 0 {
				text += getText("updateall_rollbacks_header")
				for _, name := range rollbacks {
					text += getText("updateall_rollbacks_item", getIcon(name), name)
				}
				text += "\n"
			}

			if len(failures) > 0 {
				text += getText("updateall_failures_header")
				for _, name := range failures {
					text += getText("updateall_failures_item", name)
				}
			}

			sendMessageWithClose(chatID, text)
		}()

		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	if strings.HasPrefix(query.Data, "newtag_howto:") {
		parts := strings.Split(query.Data, ":")
		if len(parts) >= 4 {
			containerName := parts[1]
			oldTag := parts[2]
			newTag := parts[3]

			howto := getText("newtag_howto", containerName, oldTag, newTag, newTag, containerName)

			msg := tgbotapi.NewMessage(chatID, howto)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	if strings.HasPrefix(query.Data, "newtag_update:") {
		// Format: newtag_update:containerName|oldTag|newTag|project|service
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
			// service name for docker compose commands (added in newer versions)
			service := containerName // fallback
			if len(parts) >= 5 && parts[4] != "" {
				service = parts[4]
			}

			editToLoading(chatID, query.Message.MessageID, getText("updating_to_tag", containerName, newTag))

			var out string

			if project != "" {
				// Compose service - edit compose file and run up
				_, composeFile, resolveErr := resolveComposeFile(project)
				if resolveErr != nil {
					out = fmt.Sprintf("❌ %v", resolveErr)
				} else if !serviceExistsInCompose(composeFile, service) {
					out = getText("service_not_in_compose", service)
				} else {
					// Use sed to replace the image tag in compose file
					sedCmd := fmt.Sprintf("sed -i 's|image: %s|image: %s|g' %s", oldTag, newTag, composeFile)
					sedOut, sedErr := runCmdWithTimeout(30*time.Second, "sh", "-c", sedCmd)

					if sedErr != nil {
						out = getText("error_editing_compose", sedErr.Error(), sedOut)
					} else {
						// Pull the new image first, then recreate (mirrors: docker compose pull && docker compose up -d)
						pullOut, pullErr := runComposeCmd(5*time.Minute, composeFile, "pull", service)
						if pullErr != nil {
							log.Printf("Compose pull error: %v\nOutput: %s", pullErr, pullOut)
							out = getText("error_downloading_image", pullOut)
							if len(out) > 3800 {
								out = out[:3800] + "\n...\n```"
							}
						} else {
							upOut, upErr := runComposeCmd(3*time.Minute, composeFile, "up", "-d", "--no-deps", service)
							if upErr != nil {
								log.Printf("Compose up error: %v\nOutput: %s", upErr, upOut)
								out = getText("error_updating", upOut)
								if len(out) > 3800 {
									out = out[:3800] + "\n...\n```"
								}
							} else {
								// Wait and verify status using container name (Docker API)
								time.Sleep(3 * time.Second)
								inspect, err := cli.ContainerInspect(ctx, containerName)
								if err == nil && inspect.State.Running {
									out = getText("updated_and_running", containerName, newTag)
								} else {
									out = getText("updated_but_stopped", containerName, newTag)
								}
							}
						}
					}
				}
			} else {
				// Standalone container - recreate with new image
				inspect, err := cli.ContainerInspect(ctx, containerName)
				if err != nil {
					out = getText("error_inspecting_container", err.Error())
				} else {
					// Pull new image first using Docker API
					pullResp, pullErr := cli.ImagePull(ctx, newTag, image.PullOptions{})
					if pullErr != nil {
						out = getText("error_downloading_image_short", pullErr.Error())
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
							out = getText("error_creating_container", err.Error())
						} else {
							// Start new container
							if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
								out = getText("error_starting_container", err.Error())
							} else {
								// Wait and verify status
								time.Sleep(3 * time.Second)
								inspect, err := cli.ContainerInspect(ctx, containerName)
								if err == nil && inspect.State.Running {
									out = getText("updated_and_running", containerName, newTag)
								} else {
									out = getText("updated_but_stopped", containerName, newTag)
								}
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
			go handleGrid(chatID, getText("restart_container_grid_title"), "restart", false)
		case "logs":
			go handleGrid(chatID, "📊 *Ver logs*", "logs", false)
		case "stop":
			go handleGrid(chatID, getText("stop_container_grid_title"), "stop", false)
		case "images":
			go handleImages(chatID)
		case "volumes":
			go handleVolumes(chatID)
		case "networks":
			go handleNetworks(chatID)
		case "check_updates":
			go func() {
				sendMessageWithClose(chatID, getText("searching_image_updates"))
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
			msg := tgbotapi.NewMessage(chatID, getText("inspect_image_prompt"))
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
			msg := tgbotapi.NewMessage(chatID, getText("inspect_volume_prompt"))
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
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_back"), "cmd:inspect_menu"),
			))
			msg := tgbotapi.NewMessage(chatID, getText("inspect_network_prompt"))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
			bot.Send(msg)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "restart":
		editToLoading(chatID, query.Message.MessageID, getText("restarting_named", target))
		timeout := 10
		err = cli.ContainerRestart(ctx, target, container.StopOptions{Timeout: &timeout})
		if err == nil {
			out = getText("container_restarted_named", target)
		}

	case "stop":
		editToLoading(chatID, query.Message.MessageID, getText("stopping_named", target))
		timeout := 10
		err = cli.ContainerStop(ctx, target, container.StopOptions{Timeout: &timeout})
		if err == nil {
			out = getText("container_stopped_named", target)
		}

	case "start":
		editToLoading(chatID, query.Message.MessageID, getText("starting_named", target))
		err = cli.ContainerStart(ctx, target, container.StartOptions{})
		if err == nil {
			time.Sleep(2 * time.Second)
			inspect, _ := cli.ContainerInspect(ctx, target)
			if inspect.State.Running {
				stats := getStats()
				stat := stats[target]
				icon := getIcon(target)
				out = getText("container_started_with_stats", icon, target, stat.CPU, stat.Mem)
			} else {
				startLogs := readContainerLogs(ctx, cli, target, container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Tail:       "20",
				})
				icon := getIcon(target)
				out = getText("container_failed_to_start", icon, target, inspect.State.Status, startLogs)
			}
		}

	case "remove":
		editToLoading(chatID, query.Message.MessageID, getText("removing_named", target))
		err = cli.ContainerRemove(ctx, target, container.RemoveOptions{})
		if err == nil {
			out = getText("container_removed_named", target)
		} else {
			msg := tgbotapi.NewMessage(chatID, getText("could_not_delete_confirm_force", target, err.Error()))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_force_delete"), "remove_force:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "close"),
				),
			)
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}

	case "remove_force":
		editToLoading(chatID, query.Message.MessageID, getText("forcing_deletion", target))
		err = cli.ContainerRemove(ctx, target, container.RemoveOptions{Force: true, RemoveVolumes: true})
		if err == nil {
			out = getText("container_force_removed", target)
		}

	case "pause":
		err = cli.ContainerPause(ctx, target)
		if err == nil {
			out = getText("container_paused_named", target)
		}

	case "unpause":
		err = cli.ContainerUnpause(ctx, target)
		if err == nil {
			out = getText("container_resumed_named", target)
		}

	case "logs":
		logs := readContainerLogs(ctx, cli, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "30",
		})

		{
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

			logsText := strings.Join(highlighted, "\n")
			if len(logsText) > 3500 {
				logsText = logsText[:3500] + getText("logs_truncated")
			}
			if logsText == "" {
				logsText = getText("no_logs_available")
			}

			out = getText("logs_header", target, logsText)

			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔴 Errors", "logs_filter:"+target+":error"),
					tgbotapi.NewInlineKeyboardButtonData("🟡 Warnings", "logs_filter:"+target+":warn"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_more_logs"), "logs_more:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_refresh"), "logs:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_download_log"), "logfile:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
				),
			)
			msg := tgbotapi.NewMessage(chatID, out)
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = keyboard
			if _, sendErr := bot.Send(msg); sendErr != nil {
				log.Printf("[logs] send error for %s: %v | content len: %d", target, sendErr, len(out))
				// Retry without ParseMode in case Markdown is broken by log content
				msg2 := tgbotapi.NewMessage(chatID, getText("logs_header_plain", target, logsText))
				msg2.ReplyMarkup = keyboard
				bot.Send(msg2)
			}
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}

	case "logs_filter":
		parts := strings.SplitN(target, ":", 2)
		if len(parts) == 2 {
			containerName, filter := parts[0], parts[1]
			logsText := readContainerLogs(ctx, cli, containerName, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Tail:       "100",
			})
			lines := strings.Split(logsText, "\n")
			filtered := []string{}
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), filter) {
					filtered = append(filtered, line)
				}
			}
			if len(filtered) > 0 {
				logsText := strings.Join(filtered, "\n")
				if len(logsText) > 3500 {
					logsText = logsText[:3500] + getText("logs_truncated")
				}
				out = getText("logs_filtered_header", filter, containerName, logsText)
			} else {
				out = getText("no_logs_found_with", filter)
			}
		}

	case "logs_more":
		logsText := readContainerLogs(ctx, cli, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "100",
		})
		if len(logsText) > 3500 {
			logsText = logsText[:3500] + getText("logs_truncated_use_download")
		}
		if logsText == "" {
			logsText = getText("no_logs_available")
		}
		out = getText("logs_full_header", target, logsText)

	case "logfile":
		logsText := readContainerLogs(ctx, cli, target, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "1000",
		})
		if logsText != "" {
			filename := fmt.Sprintf("/tmp/%s_%d.log", target, time.Now().Unix())
			os.WriteFile(filename, []byte(logsText), 0644)
			doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filename))
			doc.Caption = getText("logfile_caption", target)
			doc.ParseMode = "Markdown"
			bot.Send(doc)
			os.Remove(filename)
			bot.Request(tgbotapi.NewCallback(query.ID, getText("file_generated")))
			return
		}

	case "inspect":
		inspect, _ := cli.ContainerInspect(ctx, target)
		jsonData, _ := json.MarshalIndent(inspect, "", "  ")
		out = string(jsonData)
		if len(out) > 3800 {
			out = out[:3800] + "\n...\n(truncado)"
		}
		msg := tgbotapi.NewMessage(chatID, getText("inspect_header", target, out))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		msg := tgbotapi.NewMessage(chatID, getText("inspect_image_header", out))
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
		msg := tgbotapi.NewMessage(chatID, getText("inspect_volume_header", out))
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
		msg := tgbotapi.NewMessage(chatID, getText("inspect_network_header", out))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "rmi":
		_, err = cli.ImageRemove(ctx, target, image.RemoveOptions{})
		if err == nil {
			out = getText("image_deleted")
		}

	case "rmvol":
		err = cli.VolumeRemove(ctx, target, false)
		if err == nil {
			out = getText("volume_deleted")
		}

	case "rmnet":
		err = cli.NetworkRemove(ctx, target)
		if err == nil {
			out = getText("network_deleted")
		}

	case "prune":
		log.Printf("[prune] Starting prune operation, target: %s, chatID: %d", target, chatID)
		editToLoading(chatID, query.Message.MessageID, getText("cleaning_named", target))

		switch target {
		case "images":
			log.Printf("[prune] Pruning dangling images...")
			report, err := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
			if err == nil {
				sizeMB := float64(report.SpaceReclaimed) / 1024 / 1024
				log.Printf("[prune] Images pruned successfully: %.2f MB reclaimed", sizeMB)
				out = getText("images_pruned", fmt.Sprintf("%.2f", sizeMB))
			} else {
				log.Printf("[prune] ERROR pruning images: %v", err)
				out = getText("error_prefixed", truncateError(err, 100))
			}
		case "volumes":
			log.Printf("[prune] Pruning volumes...")
			report, err := cli.VolumesPrune(ctx, filters.Args{})
			if err == nil {
				sizeMB := float64(report.SpaceReclaimed) / 1024 / 1024
				log.Printf("[prune] Volumes pruned successfully: %.2f MB reclaimed", sizeMB)
				out = getText("volumes_pruned", fmt.Sprintf("%.2f", sizeMB))
			} else {
				log.Printf("[prune] ERROR pruning volumes: %v", err)
				out = getText("error_prefixed", truncateError(err, 100))
			}
		case "networks":
			log.Printf("[prune] Pruning networks...")
			report, err := cli.NetworksPrune(ctx, filters.Args{})
			if err == nil {
				log.Printf("[prune] Networks pruned successfully: %d networks deleted", len(report.NetworksDeleted))
				out = getText("networks_pruned", len(report.NetworksDeleted))
			} else {
				log.Printf("[prune] ERROR pruning networks: %v", err)
				out = getText("error_prefixed", truncateError(err, 100))
			}
		case "all":
			log.Printf("[prune] Pruning all resources...")

			log.Printf("[prune] Step 1/3: Pruning images...")
			imgReport, imgErr := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
			if imgErr != nil {
				log.Printf("[prune] ERROR pruning images: %v", imgErr)
			} else {
				log.Printf("[prune] Images: %.2f MB reclaimed", float64(imgReport.SpaceReclaimed)/1024/1024)
			}

			log.Printf("[prune] Step 2/3: Pruning volumes...")
			volReport, volErr := cli.VolumesPrune(ctx, filters.Args{})
			if volErr != nil {
				log.Printf("[prune] ERROR pruning volumes: %v", volErr)
			} else {
				log.Printf("[prune] Volumes: %.2f MB reclaimed", float64(volReport.SpaceReclaimed)/1024/1024)
			}

			log.Printf("[prune] Step 3/3: Pruning networks...")
			netReport, netErr := cli.NetworksPrune(ctx, filters.Args{})
			if netErr != nil {
				log.Printf("[prune] ERROR pruning networks: %v", netErr)
			} else {
				log.Printf("[prune] Networks: %d deleted", len(netReport.NetworksDeleted))
			}

			totalSpace := float64(0)
			if imgErr == nil {
				totalSpace += float64(imgReport.SpaceReclaimed)
			}
			if volErr == nil {
				totalSpace += float64(volReport.SpaceReclaimed)
			}

			totalMB := totalSpace / 1024 / 1024
			networks := 0
			if netErr == nil {
				networks = len(netReport.NetworksDeleted)
			}

			log.Printf("[prune] All resources pruned: %.2f MB total, %d networks", totalMB, networks)

			// Show errors if any
			if imgErr != nil || volErr != nil || netErr != nil {
				errMsg := "⚠️ Limpieza completada con errores:\n\n"
				if imgErr != nil {
					errMsg += getText("prune_images_error", truncateError(imgErr, 50))
				}
				if volErr != nil {
					errMsg += getText("prune_volumes_error", truncateError(volErr, 50))
				}
				if netErr != nil {
					errMsg += getText("prune_networks_error", truncateError(netErr, 50))
				}
				errMsg += getText("prune_space_freed_networks", fmt.Sprintf("%.2f", totalMB), networks)
				out = errMsg
			} else {
				out = getText("system_cleaned", fmt.Sprintf("%.2f", totalMB), networks)
			}
		}

	case "env":
		inspect, _ := cli.ContainerInspect(ctx, target)
		envVars := strings.Join(inspect.Config.Env, "\n")
		if envVars != "" {
			if len(envVars) > 3800 {
				envVars = envVars[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, getText("env_vars_header", target, envVars))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
				),
			)
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		} else {
			out = getText("no_env_vars", target)
		}

	case "update_recreate":
		editToLoading(chatID, query.Message.MessageID, getText("recreating_with_new_image", target))
		err = recreateWithNewImage(target)
		if err == nil {
			out = getText("recreated_with_new_image", target)
		}

	case "diagnose_recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Recreando *%s*...", target))
		err = recreateWithNewImage(target)
		if err == nil {
			// Wait and verify
			time.Sleep(3 * time.Second)
			inspect, err := cli.ContainerInspect(ctx, target)
			if err == nil && inspect.State.Running {
				out = getText("recreated_running", target)
			} else {
				out = getText("recreated_but_stopped", target)
			}
		}

	case "compose_pullup_service":
		// Format: project:service:containerName (containerName may be omitted for backward compat)
		parts := strings.SplitN(target, ":", 3)
		if len(parts) < 2 {
			out = getText("invalid_format")
			break
		}
		project, service := parts[0], parts[1]
		containerName := service // fallback: use service name if no container name provided
		if len(parts) == 3 && parts[2] != "" {
			containerName = parts[2]
		}

		editToLoading(chatID, query.Message.MessageID, getText("updating_named", service))

		_, composeFile, resolveErr := resolveComposeFile(project)
		if resolveErr != nil {
			out = fmt.Sprintf("❌ %v", resolveErr)
			break
		}

		if !serviceExistsInCompose(composeFile, service) {
			out = getText("service_not_in_compose", service)
			break
		}

		log.Printf("Updating service %s (container: %s) in project %s with file: %s", service, containerName, project, composeFile)

		// Up -d with pull always and no-deps (timeout 5 minutos)
		upOut, upErr := runComposeCmd(5*time.Minute, composeFile, "up", "-d", "--pull", "always", "--no-deps", service)
		if upErr != nil {
			log.Printf("Compose up error for %s: %v\nOutput: %s", service, upErr, upOut)

			// Check if it's a local image (no pull needed)
			isLocalImageError := strings.Contains(upOut, "pull access denied") ||
				strings.Contains(upOut, "repository does not exist")

			if isLocalImageError {
				log.Printf("Local image detected for %s, retrying without pull", service)
				// Retry without --pull for local images
				upOut, upErr = runComposeCmd(3*time.Minute, composeFile, "up", "-d", service)
			}

			if upErr != nil {
				out = getText("error_updating", upOut)
				if len(out) > 3800 {
					out = out[:3800] + "\n...\n```"
				}
				break
			}
		}

		log.Printf("Successfully updated service: %s", service)

		// Wait and verify status using container name (Docker API)
		time.Sleep(3 * time.Second)
		inspect, err := cli.ContainerInspect(ctx, containerName)
		if err == nil && inspect.State.Running {
			out = getText("service_updated_running", service)
		} else {
			out = getText("service_updated_but_stopped", service)
		}

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
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+target),
					tgbotapi.NewInlineKeyboardButtonData("💾 Logfile", "logfile:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_restart"), "restart:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_stop"), "stop:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔧 Env", "env:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove"), "remove_confirm:"+target),
				),
			}
		} else if inspect.State.Paused {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_resume"), "unpause:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect:"+target),
				),
			}
		} else {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_start"), "start:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_inspect"), "inspect:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+target),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove"), "remove_confirm:"+target),
				),
			}
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		))
		msg := tgbotapi.NewMessage(chatID, getText("resource_action_prompt", statusIcon, icon, target, inspect.State.Status))
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
			confirmText = getText("confirm_delete_container", target)
			confirmAction = "remove:" + target
		case "rmvol_confirm":
			confirmText = getText("confirm_delete_volume", target)
			confirmAction = "rmvol:" + target
		case "rmnet_confirm":
			confirmText = getText("confirm_delete_network", target)
			confirmAction = "rmnet:" + target
		case "rmi_confirm":
			confirmText = getText("confirm_delete_image", target)
			confirmAction = "rmi:" + target
		}
		msg := tgbotapi.NewMessage(chatID, confirmText)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_yes_delete"), confirmAction),
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
		stateMutex.Lock()
		for _, fav := range favorites[userID] {
			if fav == target {
				found = true
			} else {
				newFavs = append(newFavs, fav)
			}
		}
		if found {
			favorites[userID] = newFavs
			out = getText("removed_from_favorites", target)
		} else {
			favorites[userID] = append(favorites[userID], target)
			out = fmt.Sprintf("✅ *%s* agregado a favoritos", target)
		}
		stateMutex.Unlock()
		go handleAddFavoriteMenu(chatID, userID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Recreando *%s*...", target))
		if err2 := recreateContainer(target); err2 != nil {
			out = "❌ Error: " + err2.Error()
		} else {
			out = getText("recreated_with_new_image_alt", target)
		}

	case "backup":
		go func(vol string) {
			loadingID := sendLoading(chatID, getText("creating_volume_backup", vol))
			filename := fmt.Sprintf("/tmp/backup_%s_%d.tar.gz", vol, time.Now().Unix())
			_, err := runCmd("docker", "run", "--rm",
				"-v", vol+":/data:ro",
				"-v", "/tmp:/backup",
				"alpine", "tar", "czf", "/backup/"+strings.TrimPrefix(filename, "/tmp/"), "-C", "/data", ".")
			deleteMsg(chatID, loadingID)
			if err != nil {
				sendMessageWithClose(chatID, getText("error_creating_backup", err.Error()))
				return
			}
			doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filename))
			doc.Caption = getText("volume_backup_caption", vol)
			doc.ParseMode = "Markdown"
			bot.Send(doc)
			os.Remove(filename)
		}(target)
		bot.Request(tgbotapi.NewCallback(query.ID, getText("generating_backup")))
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
			name := containerFirstName(c)
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
		bot.Request(tgbotapi.NewCallback(query.ID, getText("config_saved")))
		return

	case "au_save":
		saveConfig()
		go handleAutoUpdate(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, getText("config_saved")))
		return

	case "track_add":
		msg := tgbotapi.NewMessage(chatID, getText("trackimage_add_prompt"))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "close"),
			),
		)
		bot.Send(msg)
		stateMutex.Lock()
		userState[query.From.ID] = "waiting_track_image"
		stateMutex.Unlock()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "track_remove":
		if len(trackedImages) == 0 {
			bot.Request(tgbotapi.NewCallback(query.ID, getText("no_tracked_images_alert")))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for img := range trackedImages {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ "+img, "track_del:"+img),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_back"), "cmd:trackimage"),
		))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, getText("trackimage_remove_prompt"))
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		bot.Send(edit)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "track_del":
		delete(trackedImages, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallback(query.ID, getText("image_removed")))
		go handleTrackImage(chatID)
		return

	case "track_check":
		go func() {
			bot.Request(tgbotapi.NewCallback(query.ID, getText("checking_alert")))
			checkTrackedImages(chatID, true)
		}()
		return

	case "chart_add":
		msg := tgbotapi.NewMessage(chatID, getText("trackchart_add_prompt"))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "close"),
			),
		)
		bot.Send(msg)
		stateMutex.Lock()
		userState[query.From.ID] = "waiting_track_chart"
		stateMutex.Unlock()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "chart_remove":
		if len(trackedCharts) == 0 {
			bot.Request(tgbotapi.NewCallback(query.ID, getText("no_tracked_charts_alert")))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for chart := range trackedCharts {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑️ "+chart, "chart_del:"+chart),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_back"), "cmd:trackchart"),
		))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, getText("trackchart_remove_prompt"))
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		bot.Send(edit)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "chart_del":
		delete(trackedCharts, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallback(query.ID, getText("chart_removed")))
		go handleTrackChart(chatID)
		return

	case "chart_check":
		go func() {
			bot.Request(tgbotapi.NewCallback(query.ID, getText("checking_alert")))
			checkTrackedCharts(chatID, true)
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
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("no_rollback_history")))
			return
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for i, entry := range history {
			label := getText("rollback_entry_label", entry.Image, entry.Timestamp.Format("02/01 15:04"))
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("rollback_do:%s|%d", target, i)),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "close"),
		))
		msg := tgbotapi.NewMessage(chatID, getText("rollback_select_version", target))
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
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("invalid_entry")))
			return
		}
		entry := history[idx]
		editToLoading(chatID, query.Message.MessageID, getText("rolling_back_to", containerName, entry.Image))
		go func() {
			if err := doRollback(containerName, entry); err != nil {
				sendMessageWithClose(chatID, getText("rollback_error", err.Error()))
			} else {
				sendMessageWithClose(chatID, getText("rollback_reverted", containerName, entry.Image))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "rollback_clear":
		delete(rollbackHistory, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("rollback_history_cleared")))
		return

	// ── Phase 2: Templates ────────────────────────────────────────────────
	case "tpl_save":
		go func() {
			if err := saveTemplate(target, query.From.ID); err != nil {
				sendMessageWithClose(chatID, getText("template_save_error", err.Error()))
			} else {
				sendMessageWithClose(chatID, getText("template_saved", target))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, "⏳"))
		return

	case "tpl_save_menu":
		containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
		var rows [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(containers); i += 2 {
			name1 := containerFirstName(containers[i])
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(getIcon(name1)+" "+name1, "tpl_save:"+name1),
			}
			if i+1 < len(containers) {
				name2 := containerFirstName(containers[i+1])
				row = append(row, tgbotapi.NewInlineKeyboardButtonData(getIcon(name2)+" "+name2, "tpl_save:"+name2))
			}
			rows = append(rows, row)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_back"), "cmd:templates"),
		))
		msg := tgbotapi.NewMessage(chatID, getText("template_save_select_title"))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "tpl_deploy":
		tpl, ok := templates[target]
		if !ok {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("template_not_found")))
			return
		}
		editToLoading(chatID, query.Message.MessageID, getText("deploying_template", tpl.Name))
		go func() {
			if err := deployTemplate(tpl); err != nil {
				sendMessageWithClose(chatID, getText("template_deploy_error", err.Error()))
			} else {
				sendMessageWithClose(chatID, getText("template_deployed", tpl.Name))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "tpl_delete":
		delete(templates, target)
		saveConfig()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("template_deleted")))
		go handleTemplates(chatID)
		return

	case "tpl_info":
		tpl, ok := templates[target]
		if !ok {
			bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("template_not_found")))
			return
		}
		visibility := getText("template_visibility_private")
		if tpl.IsPublic {
			visibility = getText("template_visibility_public")
		}
		text := getText("template_info_header", tpl.Name, tpl.Image, visibility, tpl.UsageCount)
		if len(tpl.Ports) > 0 {
			text += getText("template_ports_header")
			for h, c := range tpl.Ports {
				text += getText("template_port_item", h, c)
			}
		}
		if len(tpl.Volumes) > 0 {
			text += getText("template_volumes_header")
			for _, v := range tpl.Volumes {
				text += getText("template_volume_item", v)
			}
		}
		if len(tpl.Env) > 0 {
			text += getText("template_env_count", len(tpl.Env))
		}
		text += getText("template_created_at", tpl.CreatedAt.Format("02/01/2006 15:04"))

		visibilityBtn := tgbotapi.NewInlineKeyboardButtonData(getText("btn_make_public"), "tpl_public:"+target)
		if tpl.IsPublic {
			visibilityBtn = tgbotapi.NewInlineKeyboardButtonData(getText("btn_make_private"), "tpl_private:"+target)
		}

		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_deploy"), "tpl_deploy:"+target),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_delete"), "tpl_delete:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				visibilityBtn,
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_back"), "cmd:templates"),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "tpl_public":
		configMutex.Lock()
		if tpl, ok := templates[target]; ok {
			tpl.IsPublic = true
			templates[target] = tpl
			writeConfigLocked()
		}
		configMutex.Unlock()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("template_now_public")))
		go func() {
			time.Sleep(500 * time.Millisecond)
			handleCallback(query) // Refresh the view
		}()
		return

	case "tpl_private":
		configMutex.Lock()
		if tpl, ok := templates[target]; ok {
			tpl.IsPublic = false
			templates[target] = tpl
			writeConfigLocked()
		}
		configMutex.Unlock()
		bot.Request(tgbotapi.NewCallbackWithAlert(query.ID, getText("template_now_private")))
		go func() {
			time.Sleep(500 * time.Millisecond)
			handleCallback(query) // Refresh the view
		}()
		return

	// ── Phase 2: Maintenance mode ─────────────────────────────────────────
	case "maintenance_on":
		editToLoading(chatID, query.Message.MessageID, getText("activating_maintenance"))
		go func() {
			count, err := activateMaintenance()
			if err != nil {
				sendMessageWithClose(chatID, getText("generic_error", err.Error()))
			} else {
				sendMessageWithClose(chatID, getText("maintenance_activated", count))
			}
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	case "maintenance_off":
		editToLoading(chatID, query.Message.MessageID, getText("deactivating_maintenance"))
		go func() {
			count, err := deactivateMaintenance()
			if err != nil {
				sendMessageWithClose(chatID, getText("generic_error", err.Error()))
			} else {
				sendMessageWithClose(chatID, getText("maintenance_deactivated", count))
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
		sendMessageWithClose(chatID, getText("reports_configured_daily"))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_weekly":
		reportSchedule = "weekly"
		saveConfig()
		sendMessageWithClose(chatID, getText("reports_configured_weekly"))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_disabled":
		reportSchedule = "disabled"
		saveConfig()
		sendMessageWithClose(chatID, getText("reports_disabled"))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "report_now":
		// Trigger immediate report
		lastReportTime = time.Time{}
		sendMessageWithClose(chatID, getText("generating_report"))
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
		sendMessageWithClose(chatID, getText("audit_log_cleared"))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return

	// Phase 4 callbacks
	case "cleanup_all":
		editToLoading(chatID, query.Message.MessageID, getText("cleaning_orphan_images"))
		go func() {
			ctx := context.Background()
			report, err := cli.ImagesPrune(ctx, filters.Args{})
			if err != nil {
				sendMessageWithClose(chatID, getText("generic_error", err.Error()))
				return
			}
			sizeMB := float64(report.SpaceReclaimed) / 1024 / 1024
			sizeText := fmt.Sprintf("%.1f MB", sizeMB)
			if sizeMB > 1024 {
				sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
			}
			sendMessageWithClose(chatID, getText("cleanup_completed", sizeText))
		}()
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	// Webhook callbacks
	if query.Data == "webhook_add" {
		stateMutex.Lock()
		userState[query.From.ID] = "webhook_name"
		stateMutex.Unlock()
		sendMessageWithClose(chatID, getText("new_webhook_name_prompt"))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	if query.Data == "webhook_manual" {
		text := getText("webhook_manual_config_header")
		text += getText("webhook_manual_config_body")
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

		stateMutex.Lock()
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
		stateMutex.Unlock()

		// Update message
		text := getText("webhook_events_selected", strings.Join(newEvents, ", "))
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)
		edit.ReplyMarkup = query.Message.ReplyMarkup
		bot.Send(edit)

		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	// Save webhook
	if query.Data == "wh_save" {
		userID := query.From.ID
		stateMutex.Lock()
		data := createData[userID]
		if data == nil || data["webhook_name"] == "" || data["webhook_url"] == "" {
			stateMutex.Unlock()
			sendMessageWithClose(chatID, "❌ Error: Datos incompletos")
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}

		webhookName := data["webhook_name"]
		webhookURL := data["webhook_url"]
		eventsStr := data["webhook_events"]
		delete(userState, userID)
		delete(createData, userID)
		stateMutex.Unlock()
		if eventsStr == "" {
			eventsStr = "all"
		}

		webhooks[webhookName] = Webhook{
			URL:     webhookURL,
			Events:  strings.Split(eventsStr, ","),
			Headers: make(map[string]string),
			Enabled: true,
		}

		saveConfig()

		sendMessageWithClose(chatID, getText("webhook_created", webhookName))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}

	// Scan callback
	if strings.HasPrefix(query.Data, "scan:") {
		containerName := strings.TrimPrefix(query.Data, "scan:")
		editToLoading(chatID, query.Message.MessageID, getText("scanning_named", containerName))

		go func() {
			ctx := context.Background()
			inspect, err := cli.ContainerInspect(ctx, containerName)
			if err != nil {
				sendMessageWithClose(chatID, getText("generic_error", err.Error()))
				return
			}

			imageName := inspect.Config.Image
			result, err := scanImage(imageName)
			if err != nil {
				sendMessageWithClose(chatID, getText("scan_error", err.Error()))
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
			{tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close")},
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
						text: getText("event_container_started", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_stop"), "stop:"+name),
							),
						},
					}
				case "stop":
					n = &notification{
						text: getText("event_container_stopped", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_start"), "start:"+name),
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
							),
						},
					}
				case "die":
					exitInfo := ""
					if exitCode != "" && exitCode != "0" {
						exitInfo = getText("event_exit_code", exitCode)
					}

					lastLogs := readContainerLogs(ctx, cli, name, container.LogsOptions{
						ShowStdout: true,
						ShowStderr: true,
						Tail:       "5",
					})
					if len(lastLogs) > 500 {
						lastLogs = lastLogs[len(lastLogs)-500:]
					}

					logsSection := ""
					if lastLogs != "" {
						logsSection = getText("event_last_logs", lastLogs)
					}

					n = &notification{
						text: getText("event_container_died", icon, name, image, exitInfo, now, logsSection),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_restart"), "restart:"+name),
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
							),
						},
					}
				case "restart":
					n = &notification{
						text: getText("event_container_restarted", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_stop"), "stop:"+name),
							),
						},
					}
				case "destroy":
					n = &notification{
						text: getText("event_container_removed", icon, name, image, now),
					}
				case "pause":
					n = &notification{
						text: getText("event_container_paused", icon, name, image, now),
						buttons: [][]tgbotapi.InlineKeyboardButton{
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData(getText("btn_resume"), "unpause:"+name),
							),
						},
					}
				case "unpause":
					n = &notification{
						text: getText("event_container_resumed", icon, name, image, now),
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
			msg := getText("resource_alert", icon, name, vals.CPU, vals.Mem)
			m := tgbotapi.NewMessage(notifyChatID, msg)
			m.ParseMode = "Markdown"
			m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_restart"), "restart:"+name),
					tgbotapi.NewInlineKeyboardButtonData(getText("btn_logs"), "logs:"+name),
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

		status := getText("report_status_ok")
		if len(stoppedContainers) > 0 {
			status = getText("report_status_attention")
		}

		report := getText("report_daily", now.Format("02/01/2006"), status, len(containers), len(runningContainers), len(images), len(stoppedContainers))

		m := tgbotapi.NewMessage(notifyChatID, report)
		m.ParseMode = "Markdown"
		bot.Send(m)

		weeklyCount++
		if weeklyCount >= 7 {
			weeklyCount = 0
			volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
			networks, _ := cli.NetworkList(ctx, network.ListOptions{})

			weekly := getText("report_weekly", now.Format("02/01/2006"), status, len(containers), len(runningContainers), len(images), len(volumes.Volumes), len(networks))
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
				checkTrackedImages(notifyChatID, false)
			}
			if len(trackedCharts) > 0 {
				checkTrackedCharts(notifyChatID, false)
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
		service string // Docker Compose service name (may differ from container name)
		project string
	}
	imageMap := make(map[string][]containerInfo)

	for _, c := range containers {
		name := containerFirstName(c)
		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		project := inspect.Config.Labels["com.docker.compose.project"]
		service := inspect.Config.Labels["com.docker.compose.service"]
		if service == "" {
			service = name // fallback for standalone containers
		}
		imageTag := inspect.Config.Image // Use the tag, not the digest
		imageMap[imageTag] = append(imageMap[imageTag], containerInfo{name, service, project})
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

										msgText := getText("update_newer_tag_available", icon, strings.Join(names, "`, `"), imgTag, newerTag)

										// Add action buttons for each container
										var rows [][]tgbotapi.InlineKeyboardButton

										for _, c := range ctrs {
											rows = append(rows, tgbotapi.NewInlineKeyboardRow(
												tgbotapi.NewInlineKeyboardButtonData(getText("btn_update_named", c.name), "newtag_update:"+c.name+"|"+imgTag+"|"+newerTag+"|"+c.project+"|"+c.service),
											))
										}

										rows = append(rows, tgbotapi.NewInlineKeyboardRow(
											tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
				var recErr error
				if c.project != "" {
					// Compose container: use docker compose up (respects service name)
					_, composeFile, resolveErr := resolveComposeFile(c.project)
					if resolveErr != nil {
						recErr = resolveErr
					} else {
						out, err := runComposeCmd(5*time.Minute, composeFile, "up", "-d", "--pull", "always", "--no-deps", c.service)
						if err != nil {
							recErr = fmt.Errorf("compose up failed: %s", out)
						}
					}
				} else {
					recErr = recreateContainer(c.name)
				}
				if recErr == nil {
					autoUpdated = append(autoUpdated, c.name)
				} else {
					autoErrors = append(autoErrors, c.name+": "+recErr.Error())
				}
			}

			var msgText string
			projectLine := ""
			if len(projectSet) > 0 {
				projects := make([]string, 0, len(projectSet))
				for p := range projectSet {
					projects = append(projects, p)
				}
				projectLine = getText("update_project_line", strings.Join(projects, "`, `"))
			}

			if len(autoUpdated) > 0 {
				msgText = getText("update_auto_applied", imageTag, oldVer, newVer, sizeText, icon, strings.Join(names, "`, `"), projectLine, strings.Join(autoUpdated, "`, `"))
				if len(autoErrors) > 0 {
					msgText += getText("update_auto_errors", strings.Join(autoErrors, "; "))
				}
			} else {
				msgText = getText("update_available", imageTag, oldVer, newVer, sizeText, icon, strings.Join(names, "`, `"), projectLine)
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
						tgbotapi.NewInlineKeyboardButtonData(getText("btn_pullup_named", c.name), "compose_pullup_service:"+c.project+":"+c.service+":"+c.name),
					))
				} else {
					// Standalone container: recreate
					rows = append(rows, tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData(getText("btn_recreate_named", c.name), "update_recreate:"+c.name),
					))
				}
			}

			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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

	statusMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, getText("checking_n_containers", totalContainers)))

	found := runImageUpdateCheck()

	// Delete status message
	if statusMsg.MessageID != 0 {
		bot.Request(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
	}

	if found == 0 {
		sendMessageWithClose(chatID, getText("no_digest_updates"))
	}
}
func handleUpdateAll(chatID int64) {
	ctx := context.Background()

	// Send initial message
	statusMsg := tgbotapi.NewMessage(chatID, getText("searching_updates_listing"))
	statusMsg.ParseMode = "Markdown"
	sentMsg, _ := bot.Send(statusMsg)

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		bot.Send(tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("generic_error", err.Error())))
		return
	}

	// Update: checking images
	edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("searching_updates_checking_n", len(containers)))
	edit.ParseMode = "Markdown"
	bot.Send(edit)

	// Group containers by image
	type containerInfo struct {
		Name    string `json:"name"`
		Service string `json:"service,omitempty"` // Docker Compose service name
		Project string `json:"project"`
	}
	imageMap := make(map[string][]containerInfo)

	for _, c := range containers {
		name := containerFirstName(c)
		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		project := inspect.Config.Labels["com.docker.compose.project"]
		service := inspect.Config.Labels["com.docker.compose.service"]
		if service == "" {
			service = name // fallback for standalone containers
		}
		imageTag := inspect.Config.Image
		imageMap[imageTag] = append(imageMap[imageTag], containerInfo{Name: name, Service: service, Project: project})
	}

	// Check for updates in parallel
	type updateInfo struct {
		ImageTag   string          `json:"imageTag"`
		Containers []containerInfo `json:"containers"`
		OldID      string          `json:"oldID"`
		NewID      string          `json:"newID"`
		Size       int64           `json:"size"`
	}

	updates := []updateInfo{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	checked := 0
	totalImages := len(imageMap)

	for imageTag, ctrs := range imageMap {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(imgTag string, containers []containerInfo) {
			defer wg.Done()
			defer func() { <-semaphore }()

			inspect, _ := cli.ContainerInspect(ctx, containers[0].Name)
			localID := inspect.Image

			reader, err := cli.ImagePull(ctx, imgTag, image.PullOptions{})
			if err == nil {
				io.Copy(io.Discard, reader)
				reader.Close()
			}

			imgInspect, _, _ := cli.ImageInspectWithRaw(ctx, imgTag)
			newID := imgInspect.ID

			// Update progress
			mu.Lock()
			checked++
			currentChecked := checked
			mu.Unlock()

			edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("searching_updates_progress", currentChecked, totalImages, imgTag))
			edit.ParseMode = "Markdown"
			bot.Send(edit)

			if localID != "" && newID != "" && localID != newID {
				mu.Lock()
				updates = append(updates, updateInfo{
					ImageTag:   imgTag,
					Containers: containers,
					OldID:      localID,
					NewID:      newID,
					Size:       imgInspect.Size,
				})
				mu.Unlock()
			}
		}(imageTag, ctrs)
	}

	wg.Wait()
	bot.Send(tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID))

	if len(updates) == 0 {
		sendMessageWithClose(chatID, getText("all_containers_up_to_date"))
		return
	}

	// Build confirmation message with detailed info
	text := getText("updateall_header", len(updates))

	totalContainers := 0
	for _, upd := range updates {
		totalContainers += len(upd.Containers)

		// Format size
		sizeMB := float64(upd.Size) / 1024 / 1024
		sizeText := fmt.Sprintf("%.1f MB", sizeMB)
		if sizeMB > 1024 {
			sizeText = fmt.Sprintf("%.2f GB", sizeMB/1024)
		}

		// Short digests
		oldShort := upd.OldID
		if len(oldShort) > 19 {
			oldShort = "..." + oldShort[len(oldShort)-16:]
		}
		newShort := upd.NewID
		if len(newShort) > 19 {
			newShort = "..." + newShort[len(newShort)-16:]
		}

		containerNames := []string{}
		for _, c := range upd.Containers {
			icon := getIcon(c.Name)
			if c.Project != "" {
				containerNames = append(containerNames, getText("container_name_compose_suffix", icon, c.Name))
			} else {
				containerNames = append(containerNames, fmt.Sprintf("%s %s", icon, c.Name))
			}
		}

		text += getText("updateall_item", upd.ImageTag, oldShort, newShort, sizeText, strings.Join(containerNames, "\n      • "))
	}

	text += getText("updateall_total", totalContainers)
	text += getText("updateall_warning")

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_confirm_update"), "updateall_confirm"),
			tgbotapi.NewInlineKeyboardButtonData(getText("button_cancel"), "close"),
		),
	)
	bot.Send(msg)

	// Store updates for the callback
	stateMutex.Lock()
	if createData[chatID] == nil {
		createData[chatID] = make(map[string]string)
	}
	updatesJSON, _ := json.Marshal(updates)
	createData[chatID]["pending_updates"] = string(updatesJSON)
	stateMutex.Unlock()
}

func handleAutoUpdate(chatID int64) {
	enabled := []string{}
	for name := range autoUpdateContainers {
		if autoUpdateContainers[name] {
			enabled = append(enabled, name)
		}
	}

	text := getText("autoupdate_header")
	if len(enabled) == 0 {
		text += getText("autoupdate_none_configured")
	} else {
		text += getText("autoupdate_active_list", strings.Join(enabled, "`, `"))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_add_containers"), "au_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove_containers"), "au_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
			name := containerFirstName(containers[j])
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
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_all"), "au_all_add:_"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_none"), "au_none_add:_"),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove_all"), "au_all_rem:_"),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_save"), "au_save:"+mode),
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	text := getText("autoupdate_selector_header")
	if mode == "au_toggle_add" {
		text += getText("autoupdate_toggle_hint")
	} else {
		text += getText("autoupdate_remove_hint")
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

	text := getText("trackimage_header")
	if len(tracked) == 0 {
		text += getText("trackimage_none")
	} else {
		text += getText("trackimage_list_header")
		for _, img := range tracked {
			digest := trackedImages[img]
			shortDigest := digest
			if len(shortDigest) > 19 {
				shortDigest = "..." + shortDigest[len(shortDigest)-16:]
			}
			text += getText("trackimage_item", img, shortDigest)
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_add_image"), "track_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove_image"), "track_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_check_now"), "track_check:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		sendMessageWithClose(chatID, getText("empty_image_name"))
		return
	}

	ctx := context.Background()
	loadingID := sendLoading(chatID, getText("checking_image", imageTag))

	reader, err := cli.ImagePull(ctx, imageTag, image.PullOptions{})
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, getText("error_checking_image", err.Error()))
		return
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	imgInspect, _, err := cli.ImageInspectWithRaw(ctx, imageTag)
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, getText("error_inspecting_image", err.Error()))
		return
	}

	trackedImages[imageTag] = imgInspect.ID
	saveConfig()

	deleteMsg(chatID, loadingID)
	sendMessageWithClose(chatID, getText("image_tracking_added", imageTag, imgInspect.ID[:19]))
	go handleTrackImage(chatID)
}

func checkTrackedImages(chatID int64, manual bool) {
	if len(trackedImages) == 0 {
		if manual {
			sendMessageWithClose(chatID, getText("no_tracked_images"))
		}
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

		msgText := getText("tracked_image_update_available", imageTag, oldVer, newVer, sizeText)

		m := tgbotapi.NewMessage(chatID, msgText)
		m.ParseMode = "Markdown"
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
		bot.Send(m)
	}

	if found == 0 && manual {
		sendMessageWithClose(chatID, getText("all_tracked_images_updated"))
	}
}

func handleTrackChart(chatID int64) {
	tracked := []string{}
	for chart := range trackedCharts {
		tracked = append(tracked, chart)
	}

	text := getText("trackchart_header")
	if len(tracked) == 0 {
		text += getText("trackchart_none")
	} else {
		text += getText("trackchart_list_header")
		for _, chart := range tracked {
			info := trackedCharts[chart]
			text += getText("trackchart_item", chart, info.Version, info.AppVersion, info.Repo)
			if len(info.Images) > 0 {
				text += getText("trackchart_images_header")
				for _, img := range info.Images {
					text += getText("trackchart_image_item", img)
				}
			}
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_add_chart"), "chart_add:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_remove_chart"), "chart_remove:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_check_now"), "chart_check:_"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		sendMessageWithClose(chatID, getText("empty_chart_name"))
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

	loadingID := sendLoading(chatID, getText("checking_chart", chartName))

	pkg, err := fetchArtifactHubPackage(chartName)
	if err != nil {
		deleteMsg(chatID, loadingID)
		sendMessageWithClose(chatID, getText("error_checking_chart", err.Error()))
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
		return nil, fmt.Errorf("%s", getText("invalid_chart_format"))
	}

	url := fmt.Sprintf("https://artifacthub.io/api/v1/packages/helm/%s/%s", parts[0], parts[1])
	resp, err := exec.Command("wget", "-qO-", url).Output()
	if err != nil {
		return nil, fmt.Errorf("%s", getText("chart_not_found"))
	}

	var pkg ArtifactHubPackage
	if err := json.Unmarshal(resp, &pkg); err != nil {
		return nil, fmt.Errorf("%s", getText("parse_response_error"))
	}

	if pkg.Version == "" {
		return nil, fmt.Errorf("%s", getText("chart_not_found_no_version"))
	}

	return &pkg, nil
}

func checkTrackedCharts(chatID int64, manual bool) {
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
			appVerText = getText("chart_app_version_notification_line", pkg.AppVersion)
		}

		msgText := getText("chart_new_version", chartName, pkg.Repository.Name, oldInfo.Version, pkg.Version, appVerText)

		m := tgbotapi.NewMessage(chatID, msgText)
		m.ParseMode = "Markdown"
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_view_artifact_hub"), "chart_url:"+chartName),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
		bot.Send(m)
	}

	if found == 0 && manual {
		sendMessageWithClose(chatID, getText("all_tracked_charts_updated"))
	}
}

func handleStartContainer(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "exited")),
	})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_stopped_containers"))
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "start:"+name1),
		}
		if i+1 < len(containers) {
			name2 := containerFirstName(containers[i+1])
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "start:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("start_container_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleInspectMenu(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_containers"), "cmd:inspect_containers"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_images"), "cmd:inspect_images"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_volumes"), "cmd:inspect_volumes"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_networks"), "cmd:inspect_networks"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, getText("inspect_menu_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleStats(chatID int64) {
	loadingID := sendLoading(chatID, getText("collecting_stats"))
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

	text := getText("stats_dashboard", diskInfo, memInfo, len(containers), len(runningContainers), len(images), len(volumes.Volumes), len(networks))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("compose_projects_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handlePrune(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_images"), "prune_confirm:images"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_volumes"), "prune_confirm:volumes"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_networks"), "prune_confirm:networks"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_all_resources"), "prune_confirm:all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, getText("prune_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleExecMenu(chatID int64) {
	handleGrid(chatID, getText("exec_menu_title_grid"), "exec_menu", false)
}

func handlePauseMenu(chatID int64) {
	handleGrid(chatID, getText("pause_menu_title_grid"), "pause", false)
}

func handleUnpauseMenu(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "paused")),
	})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_paused_containers"))
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "unpause:"+name1),
		}
		if i+1 < len(containers) {
			name2 := containerFirstName(containers[i+1])
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "unpause:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("resume_container_title_alt"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}
func handleFavorites(chatID int64, userID int64) {
	stateMutex.Lock()
	favs := append([]string(nil), favorites[userID]...)
	stateMutex.Unlock()
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		sendMessageWithClose(chatID, getText("container_not_found", containerName))
		return
	}

	stateMutex.Lock()
	for _, fav := range favorites[userID] {
		if fav == containerName {
			stateMutex.Unlock()
			sendMessageWithClose(chatID, getText("already_in_favorites", containerName))
			return
		}
	}

	favorites[userID] = append(favorites[userID], containerName)
	stateMutex.Unlock()
	sendMessageWithClose(chatID, fmt.Sprintf("✅ *%s* agregado a favoritos", containerName))
}

func handleAddFavoriteMenu(chatID int64, userID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_containers"))
		return
	}

	stateMutex.Lock()
	userFavorites := append([]string(nil), favorites[userID]...)
	stateMutex.Unlock()

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
		icon1 := getIcon(name1)
		isFav1 := false
		for _, fav := range userFavorites {
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
			name2 := containerFirstName(containers[i+1])
			icon2 := getIcon(name2)
			isFav2 := false
			for _, fav := range userFavorites {
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("favorites_toggle_title_alt"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleEnvMenu(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{})

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_running_containers"))
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(containers); i += 2 {
		name1 := containerFirstName(containers[i])
		icon1 := getIcon(name1)
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+name1, "env:"+name1),
		}
		if i+1 < len(containers) {
			name2 := containerFirstName(containers[i+1])
			icon2 := getIcon(name2)
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+name2, "env:"+name2))
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("env_menu_title_alt"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleHistory(chatID int64, userID int64) {
	stateMutex.Lock()
	history := append([]string(nil), commandHistory[userID]...)
	stateMutex.Unlock()
	if len(history) == 0 {
		sendMessageWithClose(chatID, getText("no_command_history_alt"))
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
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	bot.Send(msg)
}
func handleCreateMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, getText("create_menu_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_docker_run"), "create_type:run"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_docker_compose"), "create_type:compose"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	bot.Send(msg)
}

func handleCreateRun(chatID int64, userID int64) {
	stateMutex.Lock()
	createData[userID] = make(map[string]string)
	createData[userID]["type"] = "run"
	userState[userID] = "create_image"
	stateMutex.Unlock()
	sendMessageWithClose(chatID, getText("create_run_step1"))
}

func handleCreateCompose(chatID int64, userID int64) {
	stateMutex.Lock()
	createData[userID] = make(map[string]string)
	createData[userID]["type"] = "compose"
	userState[userID] = "create_service_name"
	stateMutex.Unlock()
	sendMessageWithClose(chatID, getText("create_compose_step1"))
}

func processCreateStep(chatID int64, userID int64, text string) {
	state := userState[userID]
	data := createData[userID]

	switch state {
	case "create_image":
		data["image"] = text
		userState[userID] = "create_name"
		sendMessageWithClose(chatID, getText("create_run_step2_name"))
	case "create_name":
		if text != "/skip" {
			data["name"] = text
		}
		userState[userID] = "create_ports"
		sendMessageWithClose(chatID, getText("create_run_step3_ports"))
	case "create_ports":
		if text != "/skip" {
			data["ports"] = text
		}
		userState[userID] = "create_volumes"
		sendMessageWithClose(chatID, getText("create_step4_volumes"))
	case "create_volumes":
		if text != "/skip" {
			data["volumes"] = text
		}
		userState[userID] = "create_env"
		sendMessageWithClose(chatID, getText("create_step5_env"))
	case "create_env":
		if text != "/skip" {
			data["env"] = text
		}
		delete(userState, userID)
		generateDockerRun(chatID, userID)
	case "create_service_name":
		data["service"] = text
		userState[userID] = "create_compose_image"
		sendMessageWithClose(chatID, getText("create_compose_step2_image"))
	case "create_compose_image":
		data["image"] = text
		userState[userID] = "create_compose_ports"
		sendMessageWithClose(chatID, getText("create_compose_step3_ports"))
	case "create_compose_ports":
		if text != "/skip" {
			data["ports"] = text
		}
		userState[userID] = "create_compose_volumes"
		sendMessageWithClose(chatID, getText("create_step4_volumes"))
	case "create_compose_volumes":
		if text != "/skip" {
			data["volumes"] = text
		}
		userState[userID] = "create_compose_env"
		sendMessageWithClose(chatID, getText("create_compose_step5_env"))
	case "create_compose_env":
		if text != "/skip" {
			data["env"] = text
		}
		delete(userState, userID)
		generateDockerCompose(chatID, userID)
	}
}

func generateDockerRun(chatID int64, userID int64) {
	stateMutex.Lock()
	data := make(map[string]string, len(createData[userID]))
	for k, v := range createData[userID] {
		data[k] = v
	}
	stateMutex.Unlock()
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
	stateMutex.Lock()
	delete(createData, userID)
	stateMutex.Unlock()
}

func generateDockerCompose(chatID int64, userID int64) {
	stateMutex.Lock()
	data := make(map[string]string, len(createData[userID]))
	for k, v := range createData[userID] {
		data[k] = v
	}
	stateMutex.Unlock()
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
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	bot.Send(msg)
	delete(createData, userID)
}
func handleDiagnose(chatID int64) {
	ctx := context.Background()
	log.Printf("[diagnose] Starting diagnosis for chatID: %d", chatID)

	// Send initial message
	statusMsg := tgbotapi.NewMessage(chatID, getText("diagnose_analyzing_containers"))
	statusMsg.ParseMode = "Markdown"
	sentMsg, err := bot.Send(statusMsg)
	if err != nil {
		log.Printf("[diagnose] ERROR sending initial message: %v", err)
		return
	}
	log.Printf("[diagnose] Initial message sent, ID: %d", sentMsg.MessageID)

	issues := []string{}
	stoppedContainers := []string{}
	unhealthyContainers := []string{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Check 1: Stopped containers
	log.Printf("[diagnose] Check 1: Looking for stopped containers")
	wg.Add(1)
	go func() {
		defer wg.Done()
		stopped, err := cli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("status", "exited")),
		})
		if err != nil {
			log.Printf("[diagnose] ERROR listing stopped containers: %v", err)
			return
		}
		log.Printf("[diagnose] Found %d stopped containers", len(stopped))
		if len(stopped) > 0 {
			mu.Lock()
			for _, c := range stopped {
				name := containerFirstName(c)
				stoppedContainers = append(stoppedContainers, name)
			}
			issues = append(issues, getText("diagnose_stopped_count", len(stopped)))
			mu.Unlock()
		}
	}()

	// Update: checking health
	edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("diagnose_checking_health"))
	edit.ParseMode = "Markdown"
	bot.Send(edit)

	// Check 2: Unhealthy containers
	log.Printf("[diagnose] Check 2: Looking for unhealthy containers")
	wg.Add(1)
	go func() {
		defer wg.Done()
		running, err := cli.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			log.Printf("[diagnose] ERROR listing running containers: %v", err)
			return
		}
		log.Printf("[diagnose] Checking health of %d running containers", len(running))
		for _, c := range running {
			name := containerFirstName(c)
			inspect, err := cli.ContainerInspect(ctx, c.ID)
			if err != nil {
				log.Printf("[diagnose] ERROR inspecting %s: %v", name, err)
				continue
			}
			// Check health
			if inspect.State.Health != nil && inspect.State.Health.Status == "unhealthy" {
				log.Printf("[diagnose] Container %s is unhealthy", name)
				mu.Lock()
				unhealthyContainers = append(unhealthyContainers, name)
				escapedName := strings.ReplaceAll(name, "_", "\\_")
				issues = append(issues, getText("diagnose_unhealthy", escapedName))
				mu.Unlock()
			}
			// Check restart count
			if inspect.RestartCount > 5 {
				log.Printf("[diagnose] Container %s has %d restarts", name, inspect.RestartCount)
				mu.Lock()
				if !contains(unhealthyContainers, name) {
					unhealthyContainers = append(unhealthyContainers, name)
				}
				escapedName := strings.ReplaceAll(name, "_", "\\_")
				issues = append(issues, getText("diagnose_restarted_n_times", escapedName, inspect.RestartCount))
				mu.Unlock()
			}
		}
	}()

	// Update: checking CPU
	edit = tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("diagnose_analyzing_cpu"))
	edit.ParseMode = "Markdown"
	bot.Send(edit)

	// Check 3: High CPU usage
	log.Printf("[diagnose] Check 3: Analyzing CPU usage")
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats := getStats()
		log.Printf("[diagnose] Got stats for %d containers", len(stats))
		for name, stat := range stats {
			var cpu float64
			fmt.Sscanf(strings.TrimSuffix(stat.CPU, "%"), "%f", &cpu)
			if cpu > 80 {
				log.Printf("[diagnose] High CPU: %s at %.2f%%", name, cpu)
				mu.Lock()
				issues = append(issues, getText("diagnose_high_cpu", name, stat.CPU))
				mu.Unlock()
			}
		}
	}()

	// Update: checking images
	edit = tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, getText("diagnose_checking_images"))
	edit.ParseMode = "Markdown"
	bot.Send(edit)

	// Check 4: Dangling images
	log.Printf("[diagnose] Check 4: Looking for dangling images")
	wg.Add(1)
	go func() {
		defer wg.Done()
		danglingImages, err := cli.ImageList(ctx, image.ListOptions{
			Filters: filters.NewArgs(filters.Arg("dangling", "true")),
		})
		if err != nil {
			log.Printf("[diagnose] ERROR listing dangling images: %v", err)
			return
		}
		log.Printf("[diagnose] Found %d dangling images", len(danglingImages))
		if len(danglingImages) > 0 {
			mu.Lock()
			issues = append(issues, getText("diagnose_dangling_images", len(danglingImages)))
			mu.Unlock()
		}
	}()

	log.Printf("[diagnose] Waiting for all checks to complete...")
	wg.Wait()
	log.Printf("[diagnose] All checks completed. Found %d issues", len(issues))

	// Delete progress message
	bot.Send(tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID))

	if len(issues) == 0 {
		log.Printf("[diagnose] No issues found, sending success message")
		sendMessageWithClose(chatID, getText("diagnose_all_good"))
		return
	}

	log.Printf("[diagnose] Building report with %d stopped and %d unhealthy containers", len(stoppedContainers), len(unhealthyContainers))
	text := getText("diagnose_report_header", len(issues), strings.Join(issues, "\n"))

	var rows [][]tgbotapi.InlineKeyboardButton

	// Add buttons for stopped containers
	if len(stoppedContainers) > 0 {
		text += getText("stopped_containers_section")
		for _, name := range stoppedContainers {
			// Escape special markdown characters
			escapedName := strings.ReplaceAll(name, "_", "\\_")
			escapedName = strings.ReplaceAll(escapedName, "*", "\\*")
			escapedName = strings.ReplaceAll(escapedName, "[", "\\[")
			escapedName = strings.ReplaceAll(escapedName, "`", "\\`")
			text += fmt.Sprintf("• `%s`\n", escapedName)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_start_named", name), "start:"+name),
			))
		}
	}

	// Add buttons for unhealthy containers
	if len(unhealthyContainers) > 0 {
		text += getText("unhealthy_containers_section")
		for _, name := range unhealthyContainers {
			// Escape special markdown characters
			escapedName := strings.ReplaceAll(name, "_", "\\_")
			escapedName = strings.ReplaceAll(escapedName, "*", "\\*")
			escapedName = strings.ReplaceAll(escapedName, "[", "\\[")
			escapedName = strings.ReplaceAll(escapedName, "`", "\\`")
			text += fmt.Sprintf("• `%s`\n", escapedName)
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_recreate_diagnose", name), "diagnose_recreate:"+name),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🗑️ Prune", "cmd:prune_menu"),
		tgbotapi.NewInlineKeyboardButtonData("📋 Lista", "cmd:list"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	sent, err := bot.Send(msg)
	if err != nil {
		log.Printf("[diagnose] ERROR sending final report: %v", err)
	} else {
		log.Printf("[diagnose] Final report sent successfully, ID: %d", sent.MessageID)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func handleUptime(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	if len(containers) == 0 {
		sendMessageWithClose(chatID, getText("no_running_containers"))
		return
	}

	text := getText("uptime_title")
	for _, c := range containers {
		name := containerFirstName(c)
		icon := getIcon(name)
		text += fmt.Sprintf("%s *%s*\n   └ `%s`\n", icon, name, c.Status)
	}
	sendMessageWithClose(chatID, text)
}

func handleBackupMenu(chatID int64) {
	ctx := context.Background()
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil || len(volumes.Volumes) == 0 {
		sendMessageWithClose(chatID, getText("no_volumes_available"))
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
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, getText("backup_volume_select_title"))
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

	// Initialize metrics store (keep last 10080 points = 7 days at 1 minute intervals)
	metricsStore := api.NewMetricsStore("/data/metrics.json", 10080)

	// Initialize alert store with Telegram notification callback
	alertStore := api.NewAlertStore("/data/alerts.json", func(alert api.Alert) {
		if notifyChatID == 0 {
			return
		}
		icon := "🚨"
		if alert.Type == "cpu" {
			icon = "⚠️ CPU"
		} else {
			icon = "💾 RAM"
		}
		text := fmt.Sprintf("%s *Alert Triggered*\n\n"+
			"Container: `%s`\n"+
			"Type: %s\n"+
			"Threshold: %.1f%%\n"+
			"Current: %.1f%%\n"+
			"Time: %s",
			icon,
			alert.ContainerName,
			alert.Type,
			alert.Threshold,
			alert.CurrentValue,
			alert.TriggeredAt.Format("15:04:05"))

		msg := tgbotapi.NewMessage(notifyChatID, text)
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	})

	// Start metrics collector (collect every 30 seconds)
	go api.CollectMetrics(cli, metricsStore, alertStore, 30*time.Second)

	// Initialize user store
	userStore := api.NewUserStore("/data/users.json")

	// Initialize template store
	templateStore := api.NewTemplateStore("/data/templates.json")

	// Start API server for Mini App — disabled by default (security incident,
	// see CHANGELOG_v2.4.3.md). Set ENABLE_MINI_APP=true to opt back in.
	if os.Getenv("ENABLE_MINI_APP") == "true" {
		apiServer := api.NewServer(cli, metricsStore, alertStore, userStore, templateStore)
		go func() {
			if err := apiServer.Start("8080"); err != nil {
				log.Printf("API server error: %v", err)
			}
		}()
		log.Printf("Mini App API server enabled on port 8080")
	} else {
		log.Printf("Mini App API server disabled (set ENABLE_MINI_APP=true to enable)")
	}

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

	// Check for incomplete transactions and offer recovery
	if notifyChatID != 0 {
		go func() {
			time.Sleep(2 * time.Second) // Wait for bot to be fully initialized
			checkAndRecoverTransaction()
		}()
	}

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
		{Command: "updateall", Description: getText("menu_updateall")},
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
		{Command: "rollback", Description: getText("menu_rollback")},
		{Command: "templates", Description: getText("menu_templates")},
		{Command: "maintenance", Description: getText("menu_maintenance")},
		// Phase 1
		{Command: "alerts", Description: getText("menu_alerts")},
		{Command: "healthchecks", Description: getText("menu_healthchecks")},
		{Command: "reports", Description: getText("menu_reports")},
		// Phase 3
		{Command: "audit", Description: getText("menu_audit")},
		{Command: "scan", Description: getText("menu_scan")},
		{Command: "webhooks", Description: getText("menu_webhooks")},
		{Command: "policies", Description: getText("menu_policies")},
		// Phase 4
		{Command: "registries", Description: getText("menu_registries")},
		{Command: "networks", Description: getText("menu_networks_manage")},
		{Command: "cleanup", Description: getText("menu_cleanup")},
		{Command: "ports", Description: getText("menu_ports")},
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

	log.Printf("Listening for updates...")
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
				stateMutex.Lock()
				commandHistory[userID] = append(commandHistory[userID], update.Message.Command())
				if len(commandHistory[userID]) > 50 {
					commandHistory[userID] = commandHistory[userID][1:]
				}
				stateMutex.Unlock()
				// Add to audit log
				addAudit(userID, update.Message.Command(), update.Message.CommandArguments(), true)
			}

			// Delete command message
			bot.Send(tgbotapi.NewDeleteMessage(chatID, update.Message.MessageID))

			// Check user state
			stateMutex.Lock()
			state, exists := userState[userID]
			stateMutex.Unlock()
			if exists && update.Message.Command() == "" {
				text := update.Message.Text
				if strings.HasPrefix(state, "create_") {
					go processCreateStep(chatID, userID, text)
					continue
				}
				if state == "waiting_search" {
					stateMutex.Lock()
					delete(userState, userID)
					stateMutex.Unlock()
					go handleSearch(chatID, text)
					continue
				}
				if state == "waiting_track_image" {
					stateMutex.Lock()
					delete(userState, userID)
					stateMutex.Unlock()
					go addTrackedImage(chatID, text)
					continue
				}
				if state == "waiting_track_chart" {
					stateMutex.Lock()
					delete(userState, userID)
					stateMutex.Unlock()
					go addTrackedChart(chatID, text)
					continue
				}
				if state == "webhook_name" {
					stateMutex.Lock()
					if createData[userID] == nil {
						createData[userID] = make(map[string]string)
					}
					createData[userID]["webhook_name"] = text
					userState[userID] = "webhook_url"
					stateMutex.Unlock()
					sendMessageWithClose(chatID, getText("webhook_url_prompt"))
					continue
				}
				if state == "webhook_url" {
					stateMutex.Lock()
					if createData[userID] == nil {
						createData[userID] = make(map[string]string)
					}
					createData[userID]["webhook_url"] = text
					userState[userID] = "webhook_events"
					stateMutex.Unlock()
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
							tgbotapi.NewInlineKeyboardButtonData(getText("btn_all"), "wh_event:all"),
						),
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData(getText("btn_save"), "wh_save"),
						),
					)
					msg := tgbotapi.NewMessage(chatID, getText("webhook_events_prompt"))
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
				go handleGrid(chatID, getText("restart_container_grid_title"), "restart", false)
			case "stop":
				go handleGrid(chatID, getText("stop_container_grid_title"), "stop", false)
			case "logs":
				go handleGrid(chatID, "📊 *Ver logs*", "logs", false)
			case "logfile":
				go handleGrid(chatID, getText("btn_download_logs"), "logfile", false)
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
					sendMessageWithClose(chatID, getText("searching_updates_ellipsis"))
					runImageUpdateCheckWithFeedback(chatID)
				}()
			case "updateall":
				go handleUpdateAll(chatID)
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
					stateMutex.Lock()
					userState[userID] = "waiting_search"
					stateMutex.Unlock()
					sendMessageWithClose(chatID, getText("search_prompt"))
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
			case "registries":
				go handleRegistries(chatID)
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
	writeConfigLocked()
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
		sendMessageWithClose(chatID, getText("generic_error", err.Error()))
		return
	}

	// Only show containers that have rollback history
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range containers {
		name := containerFirstName(c)
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
		sendMessageWithClose(chatID, getText("rollback_no_history"))
		return
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))
	msg := tgbotapi.NewMessage(chatID, getText("rollback_select_container_title"))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 2: Container Templates
// ═══════════════════════════════════════════════════════════════════════════

func saveTemplate(containerName string, userID int64) error {
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
		IsPublic:    false,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
		UsageCount:  0,
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

	// Increment usage count
	configMutex.Lock()
	if t, exists := templates[tpl.Name]; exists {
		t.UsageCount++
		templates[tpl.Name] = t
		writeConfigLocked()
	}
	configMutex.Unlock()

	return cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
}

func handleTemplates(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	text := getText("templates_title")
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
		text += getText("no_saved_templates")
	}

	// Add "save from container" section
	if len(containers) > 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_save_as_template"), "tpl_save_menu:_"),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		name := containerFirstName(c)
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
		sendMessageWithClose(chatID, getText("no_search_results", query))
		return
	}

	text := getText("search_results_header", query, len(results), strings.Join(results, "\n\n"))
	if len(text) > 3800 {
		text = text[:3800] + "\n...(truncado)"
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
		name := containerFirstName(c)
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
	statusText := getText("maintenance_status_inactive")
	if maintenanceMode {
		statusIcon = "🔧"
		statusText = getText("maintenance_status_active")
	}

	text := getText("maintenance_mode_header", statusIcon, statusText, len(running), len(paused))

	if maintenanceMode && len(maintenancePaused) > 0 {
		text += getText("maintenance_paused_by", strings.Join(maintenancePaused, "`, `"))
	}

	var keyboard tgbotapi.InlineKeyboardMarkup
	if maintenanceMode {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_deactivate_maintenance"), "maintenance_off:_"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_refresh_status"), "maintenance_status:_"),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
			),
		)
	} else {
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_activate_maintenance"), "maintenance_on:_"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_refresh_status"), "maintenance_status:_"),
				tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
//
//	nginx → registry-1.docker.io, library/nginx
//	user/image → registry-1.docker.io, user/image
//	ghcr.io/user/image → ghcr.io, user/image
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
			name := containerFirstName(c)
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
				msg := getText("cpu_alert", name, fmt.Sprintf("%.1f", cpuPercent), fmt.Sprintf("%.0f", alert.CPUThreshold))
				bot.Send(tgbotapi.NewMessage(notifyChatID, msg))
			}

			if ramPercent > alert.RAMThreshold {
				msg := getText("ram_alert", name, fmt.Sprintf("%.1f", ramPercent), fmt.Sprintf("%.0f", alert.RAMThreshold))
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
				msg := getText("health_check_failed", name, check.Type, check.Target)
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

		msg := getText("system_report", running, stopped, paused, len(containers), now.Format("2006-01-02 15:04"))

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

	text := getText("resource_usage_title")
	hasRunning := false

	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		hasRunning = true
		name := containerFirstName(c)

		statsResp, err := cli.ContainerStats(ctx, c.ID, false)
		if err != nil {
			text += getText("error_getting_stats", name)
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

		text += getText("resource_usage_item", icon, name, fmt.Sprintf("%.1f", cpuPercent), fmt.Sprintf("%.2f", memUsage), fmt.Sprintf("%.1f", memPercent))
	}

	if !hasRunning {
		text += getText("no_running_containers_alerts")
	}

	sendMessageWithClose(chatID, text)
}

// Handle /healthchecks command
func handleHealthChecks(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	text := getText("containers_status_title")
	text += getText("healthchecks_status_header")

	running := 0
	stopped := 0
	unhealthy := 0

	for _, c := range containers {
		name := containerFirstName(c)
		icon := "🟢"
		status := "Corriendo"

		if c.State == "exited" {
			icon = "🔴"
			status = getText("status_stopped_label")
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

	text += getText("healthchecks_summary", running, stopped, unhealthy)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
		),
	)
	bot.Send(msg)
}

// Handle /reports command
func handleReports(chatID int64) {
	text := getText("reports_header", reportSchedule)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_daily"), "report_daily"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_weekly"), "report_weekly"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_disable"), "report_disabled"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_send_now"), "report_now"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
	text := getText("audit_log_header")

	if len(auditLog) == 0 {
		text += getText("audit_log_empty")
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
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_export"), "audit_export"),
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_clean"), "audit_clear"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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

	return getText("scan_result", imageName, critical, high), nil
}

// Handle /scan command
func handleScan(chatID int64) {
	ctx := context.Background()
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})

	text := getText("scan_select_container")

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range containers {
		name := containerFirstName(c)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 "+name, "scan:"+name),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
	))

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	bot.Send(msg)
}

// Handle /webhooks command
func handleWebhooks(chatID int64) {
	text := getText("webhooks_title")

	if len(webhooks) == 0 {
		text += getText("webhooks_none_configured")
		text += getText("webhooks_explanation")
		text += getText("webhooks_available_events")
		text += "• container.start\n"
		text += "• container.stop\n"
		text += "• container.die\n"
		text += "• image.update"
	} else {
		text += getText("webhooks_configured_header")
		for name, wh := range webhooks {
			status := "❌"
			if wh.Enabled {
				status = "✅"
			}
			text += getText("webhook_item", status, name, wh.URL, len(wh.Events))
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_add_webhook"), "webhook_add"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_view_manual_config"), "webhook_manual"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
	text := getText("policies_title")
	text += getText("policies_explanation")
	text += getText("policies_current_config")

	if len(autoUpdateContainers) == 0 {
		text += getText("policies_none_configured")
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

	text += getText("policies_hint")

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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
	text := getText("registries_title")

	text += getText("registries_supported")
	text += getText("registries_list")

	text += getText("registries_how_to_auth")
	text += "```bash\n"
	text += "docker login ghcr.io\n"
	text += "docker login registry.example.com\n"
	text += "```\n\n"

	text += getText("registries_hint")

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

	text := getText("cleanup_header", len(orphaned), sizeText)

	if len(orphaned) > 0 {
		text += getText("cleanup_detected_images")
		for i, img := range orphaned {
			if i >= 10 {
				text += getText("cleanup_more_items", len(orphaned)-10)
				break
			}
			text += fmt.Sprintf("• `%s`\n", img)
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_clean_all"), "cleanup_all"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(getText("btn_close"), "close"),
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

	text := getText("exposed_ports_title")

	seen := make(map[string]bool)

	for _, c := range containers {
		name := containerFirstName(c)
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
		text += getText("no_exposed_ports")
	}

	sendMessageWithClose(chatID, text)
}
