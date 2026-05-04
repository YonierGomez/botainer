package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

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
	botVersion     = "1.0.0"                      // First stable release
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
	checkUpdatesInterval    = 6 * time.Hour
	enableAutoCheck         = true
	enableStartupNotif      = true
	configMutex             sync.Mutex
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

// Config structure for persistence
type Config struct {
	AutoUpdateContainers map[string]bool `json:"autoUpdateContainers"`
	LastCheck            time.Time       `json:"lastCheck"`
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
}

// Save configuration to file
func saveConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	cfg := Config{
		AutoUpdateContainers: autoUpdateContainers,
		LastCheck:            time.Now(),
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

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return stats
	}

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		statsResp, err := cli.ContainerStats(ctx, c.ID, false)
		if err != nil {
			continue
		}
		defer statsResp.Body.Close()

		var v types.StatsJSON
		if err := json.NewDecoder(statsResp.Body).Decode(&v); err != nil {
			continue
		}

		cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
		cpuPercent := 0.0
		if systemDelta > 0 && cpuDelta > 0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
		}

		memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024
		memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024

		stats[name] = struct{ CPU, Mem string }{
			fmt.Sprintf("%.2f%%", cpuPercent),
			fmt.Sprintf("%.0fMiB / %.0fMiB", memUsage, memLimit),
		}
	}

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
			tgbotapi.NewInlineKeyboardButtonData("🔄 Update All", "cmd:updateall"),
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

func handleUpdateAll(chatID int64) {
	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	statusMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("🔄 Verificando %d contenedores...", len(containers))))

	type containerInfo struct {
		name, imageTag, oldID string
	}

	var containerList []containerInfo
	imageSet := make(map[string]bool)

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		inspect, _ := cli.ContainerInspect(ctx, c.ID)
		containerList = append(containerList, containerInfo{name, inspect.Config.Image, inspect.Image})
		imageSet[inspect.Config.Image] = true
	}

	pulledImages := make(map[string]string)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for imageTag := range imageSet {
		wg.Add(1)
		go func(img string) {
			defer wg.Done()
			reader, err := cli.ImagePull(ctx, img, image.PullOptions{})
			if err != nil {
				log.Printf("Error pulling %s: %v", img, err)
				return
			}
			io.Copy(io.Discard, reader)
			reader.Close()

			imgInspect, _, err := cli.ImageInspectWithRaw(ctx, img)
			if err != nil {
				log.Printf("Error inspecting %s: %v", img, err)
				return
			}

			mu.Lock()
			pulledImages[img] = imgInspect.ID
			mu.Unlock()
			log.Printf("Pulled %s: %s", img, imgInspect.ID[:12])
		}(imageTag)
	}

	wg.Wait()

	var updates []struct{ name, image string }
	for _, c := range containerList {
		newID := pulledImages[c.imageTag]
		if newID == "" {
			newID = c.oldID
		}

		oldShort := c.oldID
		if len(oldShort) > 12 {
			oldShort = oldShort[:12]
		}
		newShort := newID
		if len(newShort) > 12 {
			newShort = newShort[:12]
		}

		log.Printf("Container %s: old=%s new=%s", c.name, oldShort, newShort)
		if c.oldID != newID && newID != "" {
			updates = append(updates, struct{ name, image string }{c.name, c.imageTag})
		}
	}

	bot.Request(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))

	if len(updates) == 0 {
		sendMessageWithClose(chatID, "✅ Todas las imágenes están actualizadas")
		return
	}

	for _, u := range updates {
		text := fmt.Sprintf("🆕 *Actualización disponible*\n\n📦 Contenedor: `%s`\n🖼️ Imagen: `%s`", u.name, u.image)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"

		project := ""
		inspect, _ := cli.ContainerInspect(ctx, u.name)
		if p := inspect.Config.Labels["com.docker.compose.project"]; p != "" {
			project = p
		}

		var buttons []tgbotapi.InlineKeyboardButton
		if project != "" {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("🔄 Pull & Up: "+project, "compose_pullup:"+project))
		} else {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("🔄 Recrear", "update_recreate:"+u.name))
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("❌ Ignorar", "close"))

		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...))
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
				var v types.StatsJSON
				if json.NewDecoder(statsResp.Body).Decode(&v) == nil {
					cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage - v.PreCPUStats.CPUUsage.TotalUsage)
					systemDelta := float64(v.CPUStats.SystemUsage - v.PreCPUStats.SystemUsage)
					if systemDelta > 0 && cpuDelta > 0 {
						cpuPercent := (cpuDelta / systemDelta) * float64(len(v.CPUStats.CPUUsage.PercpuUsage)) * 100.0
						cpu = fmt.Sprintf("%.2f%%", cpuPercent)
					}
					memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024
					memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024
					mem = fmt.Sprintf("%.0fMiB / %.0fMiB", memUsage, memLimit)
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
		case "updateall":
			go handleUpdateAll(chatID)
		case "check_updates":
			go func() {
				sendMessageWithClose(chatID, "🔍 Buscando actualizaciones de imágenes...")
				runImageUpdateCheckWithFeedback(chatID)
			}()
		case "list":
			go handleList(chatID)
		case "diagnose":
			go handleDiagnose(chatID)
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
		err = cli.ContainerRemove(ctx, target, container.RemoveOptions{Force: true})
		if err == nil {
			out = fmt.Sprintf("✅ *%s* eliminado", target)
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

	case "compose_pullup":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Actualizando proyecto *%s*...", target))

		workDir := getComposeWorkDir(target)
		if workDir == "" {
			out = "❌ No se encontró el directorio del proyecto o archivo compose"
			break
		}

		composeFile := findComposeFile(workDir)
		if composeFile == "" {
			out = fmt.Sprintf("❌ No se encontró archivo compose en: `%s`", workDir)
			break
		}

		log.Printf("Updating compose project %s with file: %s", target, composeFile)

		// Pull images (timeout 5 minutos)
		pullOut, pullErr := runCmdWithTimeout(5*time.Minute, "docker", "compose", "-f", composeFile, "pull")
		if pullErr != nil {
			log.Printf("Compose pull error for %s: %v\nOutput: %s", target, pullErr, pullOut)
			out = fmt.Sprintf("❌ Error al hacer pull:\n```\n%s\n```", pullOut)
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n```"
			}
			break
		}

		// Up -d (timeout 3 minutos)
		upOut, upErr := runCmdWithTimeout(3*time.Minute, "docker", "compose", "-f", composeFile, "up", "-d")
		if upErr != nil {
			log.Printf("Compose up error for %s: %v\nOutput: %s", target, upErr, upOut)
			out = fmt.Sprintf("❌ Error al actualizar:\n```\n%s\n```", upOut)
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n```"
			}
			break
		}

		log.Printf("Successfully updated compose project: %s", target)
		out = fmt.Sprintf("✅ Proyecto *%s* actualizado correctamente", target)

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
			var cpu, mem float64
			fmt.Sscanf(strings.TrimSuffix(vals.CPU, "%"), "%f", &cpu)
			fmt.Sscanf(strings.Split(vals.Mem, "/")[0], "%fMiB", &mem)

			if cpu > 90 || mem > 90 {
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
		imageMap[c.Image] = append(imageMap[c.Image], containerInfo{name, project})
	}

	found := 0
	for imageTag, containers := range imageMap {
		inspect, _ := cli.ContainerInspect(ctx, containers[0].name)
		localID := inspect.Image

		reader, err := cli.ImagePull(ctx, imageTag, image.PullOptions{})
		if err == nil {
			io.Copy(io.Discard, reader)
			reader.Close()
		}

		imgInspect, _, _ := cli.ImageInspectWithRaw(ctx, imageTag)
		newID := imgInspect.ID

		if localID == "" || newID == "" || localID == newID {
			continue
		}
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

		// Group by project for compose buttons
		if len(projectSet) > 0 {
			for project := range projectSet {
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Pull & Up: "+project, "compose_pullup:"+project),
				))
			}
		}

		// Individual container buttons for non-compose or standalone
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
			if alreadyDone {
				continue
			}

			// Only show individual buttons if not compose or if user wants granular control
			if c.project == "" {
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
	}
	return found
}

func runImageUpdateCheckWithFeedback(chatID int64) {
	found := runImageUpdateCheck()
	if found == 0 {
		sendMessageWithClose(chatID, "✅ Todas las imágenes están actualizadas")
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

func handleSearch(chatID int64, query string) {
	if query == "" {
		return
	}

	ctx := context.Background()
	query = strings.ToLower(query)
	results := []string{}

	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(c.Image), query) {
			results = append(results, fmt.Sprintf("📦 %s (`%s`)", name, c.Image))
		}
	}

	images, _ := cli.ImageList(ctx, image.ListOptions{})
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if strings.Contains(strings.ToLower(tag), query) {
				results = append(results, fmt.Sprintf("🖼️ %s", tag))
				break
			}
		}
	}

	volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
	for _, vol := range volumes.Volumes {
		if strings.Contains(strings.ToLower(vol.Name), query) {
			results = append(results, fmt.Sprintf("💾 %s", vol.Name))
		}
	}

	if len(results) == 0 {
		sendMessageWithClose(chatID, fmt.Sprintf("No se encontraron resultados para: *%s*", query))
	} else {
		text := fmt.Sprintf("🔍 *Resultados para: %s*\n\n%s", query, strings.Join(results, "\n"))
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
	}
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

	stoppedContainers, _ := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("status", "exited")),
	})
	if len(stoppedContainers) > 0 {
		issues = append(issues, fmt.Sprintf("⚠️ %d contenedores detenidos", len(stoppedContainers)))
	}

	stats := getStats()
	for name, stat := range stats {
		var cpu float64
		fmt.Sscanf(strings.TrimSuffix(stat.CPU, "%"), "%f", &cpu)
		if cpu > 80 {
			issues = append(issues, fmt.Sprintf("🔥 %s usando %s CPU", name, stat.CPU))
		}
	}

	danglingImages, _ := cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("dangling", "true")),
	})
	if len(danglingImages) > 0 {
		issues = append(issues, fmt.Sprintf("🗑️ %d imágenes sin usar (ejecuta /prune)", len(danglingImages)))
	}

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

	// Validate Docker Compose availability
	if err := validateComposeSetup(); err != nil {
		log.Printf("⚠️  Warning: %v", err)
		log.Printf("⚠️  Compose features will be disabled")
	}

	log.Printf("Bot iniciado: @%s", bot.Self.UserName)

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
		startupMsg := fmt.Sprintf("🤖 *Botainer v%s*\n🟢 Activo\n\n_Bot iniciado correctamente_", botVersion)
		msg := tgbotapi.NewMessage(notifyChatID, startupMsg)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("📢 Canal de Novedades", newsChannelURL),
			),
		)
		bot.Send(msg)
	}

	// Set bot commands
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "🐳 Menú principal"},
		{Command: "version", Description: "ℹ️ Versión del bot"},
		{Command: "list", Description: "🐳 Todos los contenedores"},
		{Command: "ps", Description: "🐳 Contenedores corriendo"},
		{Command: "running", Description: "🐳 Contenedores con acciones"},
		{Command: "stats", Description: "🐳 Dashboard del sistema"},
		{Command: "create", Description: "🐳 Crear contenedor"},
		{Command: "restart", Description: "🐳 Reiniciar contenedor"},
		{Command: "stop", Description: "🐳 Detener contenedor"},
		{Command: "start_container", Description: "🐳 Iniciar contenedor"},
		{Command: "pause", Description: "🐳 Pausar contenedor"},
		{Command: "unpause", Description: "🐳 Reanudar contenedor"},
		{Command: "logs", Description: "🐳 Ver logs"},
		{Command: "logfile", Description: "🐳 Descargar logs"},
		{Command: "exec", Description: "🐳 Ejecutar comando"},
		{Command: "diagnose", Description: "🐳 Diagnóstico"},
		{Command: "compose", Description: "🐳 Docker Compose"},
		{Command: "inspect", Description: "🐳 Inspeccionar"},
		{Command: "images", Description: "🐳 Listar imágenes"},
		{Command: "volumes", Description: "🐳 Listar volúmenes"},
		{Command: "networks", Description: "🐳 Listar redes"},
		{Command: "prune", Description: "🐳 Limpiar recursos"},
		{Command: "checkupdates", Description: "🐳 Buscar actualizaciones"},
		{Command: "updateall", Description: "🐳 Actualizar todo"},
		{Command: "autoupdate", Description: "🔁 Auto-update"},
		{Command: "search", Description: "🐳 Buscar"},
		{Command: "env", Description: "🐳 Variables de entorno"},
		{Command: "favorites", Description: "🐳 Favoritos"},
		{Command: "addfav", Description: "🐳 Agregar favorito"},
		{Command: "history", Description: "🐳 Historial"},
		{Command: "uptime", Description: "🐳 Uptime"},
		{Command: "backup", Description: "🐳 Backup volumen"},
	}

	cmdConfig := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(cmdConfig); err != nil {
		log.Printf("Error setting commands: %v", err)
	}

	go monitorEvents()
	go checkUpdates()
	go monitorResourceAlerts()
	go scheduledReports()

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
			case "updateall":
				go handleUpdateAll(chatID)
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
			case "search":
				if update.Message.CommandArguments() == "" {
					userState[userID] = "waiting_search"
					sendMessageWithClose(chatID, "🔍 ¿Qué deseas buscar?")
				} else {
					go handleSearch(chatID, update.Message.CommandArguments())
				}
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
			case "uptime":
				go handleUptime(chatID)
			case "backup":
				go handleBackupMenu(chatID)
			}
		} else if update.CallbackQuery != nil {
			go handleCallback(update.CallbackQuery)
		}
	}
}
