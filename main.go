package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	bot        *tgbotapi.BotAPI
	notifyChatID int64
	allowedUsers []int64
	favorites    = make(map[int64][]string) // userID -> container names
	commandHistory = make(map[int64][]string) // userID -> commands
	userState = make(map[int64]string) // userID -> current state (waiting_search, waiting_addfav, etc)
	createData = make(map[int64]map[string]string) // userID -> container creation data
	containerIcons = map[string]string{
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

func getStats() map[string]struct{ CPU, Mem string } {
	stats := make(map[string]struct{ CPU, Mem string })
	out, err := runCmd("docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}")
	if err != nil {
		return stats
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) >= 3 {
			stats[parts[0]] = struct{ CPU, Mem string }{parts[1], parts[2]}
		}
	}
	return stats
}

func handleStart(chatID int64) {
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
	out, err := runCmd("docker", "network", "ls", "--format", "{{.Name}}|{{.Driver}}|{{.Scope}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		name, driver, scope := parts[0], parts[1], parts[2]
		
		// Get containers in this network
		containersOut, _ := runCmd("docker", "network", "inspect", name, "--format", "{{range .Containers}}{{.Name}} {{end}}")
		containers := strings.TrimSpace(containersOut)
		
		// Get compose project
		inspectOut, _ := runCmd("docker", "network", "inspect", name)
		var project string
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		
		text := fmt.Sprintf("🌐 *%s*\n   ├ Driver: `%s`\n   ├ Scope: `%s`", name, driver, scope)
		if containers != "" {
			text += fmt.Sprintf("\n   ├ Contenedores: `%s`", containers)
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
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_net:"+name),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmnet_confirm:"+name),
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
	msg := tgbotapi.NewMessage(chatID, "🔄 Actualizando todas las imágenes...")
	bot.Send(msg)
	
	// Get all running containers with their images and projects
	out, err := runCmd("docker", "ps", "--format", "{{.Names}}|{{.Image}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}
	
	projects := make(map[string]bool)
	standaloneContainers := []string{}
	
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) < 2 {
			continue
		}
		name, image := parts[0], parts[1]
		
		// Check if it's compose
		inspectOut, _ := runCmd("docker", "inspect", name)
		var project string
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Config"].(map[string]interface{})["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		
		if project != "" {
			projects[project] = true
		} else {
			standaloneContainers = append(standaloneContainers, name+"|"+image)
		}
	}
	
	// Update compose projects
	for project := range projects {
		runCmd("docker", "compose", "-p", project, "pull")
		result, _ := runCmd("docker", "compose", "-p", project, "up", "-d")
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Proyecto *%s* actualizado\n```\n%s\n```", project, result))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
	
	// Update standalone containers
	for _, container := range standaloneContainers {
		parts := strings.Split(container, "|")
		if len(parts) < 2 {
			continue
		}
		name, image := parts[0], parts[1]
		
		runCmd("docker", "pull", image)
		runCmd("docker", "stop", name)
		runCmd("docker", "rm", name)
		runCmd("docker", "run", "-d", "--name", name, image)
		
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ *%s* actualizado", name))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
	
	finalMsg := tgbotapi.NewMessage(chatID, "✅ Todas las actualizaciones completadas")
	bot.Send(finalMsg)
}

func handleImages(chatID int64) {
	out, err := runCmd("docker", "images", "--format", "{{.Repository}}:{{.Tag}}|{{.ID}}|{{.Size}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		image, id, size := parts[0], parts[1], parts[2]
		
		text := fmt.Sprintf("🖼️ *%s*\n   ├ ID: `%s`\n   └ Tamaño: `%s`", image, id[:12], size)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true
		
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_img:"+id),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmi_confirm:"+id),
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
	out, err := runCmd("docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, volumeName := range lines {
		volumeName = strings.TrimSpace(volumeName)
		if volumeName == "" {
			continue
		}
		
		// Inspect volume to get labels and find containers using it
		inspectOut, _ := runCmd("docker", "volume", "inspect", volumeName)
		
		// Find containers using this volume
		containersOut, _ := runCmd("docker", "ps", "-a", "--filter", "volume="+volumeName, "--format", "{{.Names}}")
		containers := []string{}
		for _, c := range strings.Split(strings.TrimSpace(containersOut), "\n") {
			c = strings.TrimSpace(c)
			if c != "" {
				containers = append(containers, c)
			}
		}
		
		// Extract project label from inspect
		var project string
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		
		var text string
		if len(containers) > 0 {
			text = fmt.Sprintf("💾 *%s*\n   ├ Usado por: `%s`", volumeName, strings.Join(containers, ", "))
			if project != "" {
				text += fmt.Sprintf("\n   └ Proyecto: `%s`", project)
			}
		} else if project != "" {
			text = fmt.Sprintf("💾 *%s*\n   └ Proyecto: `%s`", volumeName, project)
		} else {
			text = fmt.Sprintf("💾 *%s*\n   └ Sin usar", volumeName)
		}
		
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect_vol:"+volumeName),
				tgbotapi.NewInlineKeyboardButtonData("🗑️ Delete", "rmvol_confirm:"+volumeName),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💾 Backup", "backup:"+volumeName),
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
	out, err := runCmd("docker", "ps", "--format", "{{.Names}}|{{.Status}}|{{.Image}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	stats := getStats()
	
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		name, status, image := parts[0], parts[1], parts[2]
		icon := getIcon(name)
		stat := stats[name]
		
		// Get compose project
		inspectOut, _ := runCmd("docker", "inspect", name)
		var project string
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Config"].(map[string]interface{})["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		
		text := fmt.Sprintf("🟢 %s *%s*\n   ├ Estado: `%s`\n   ├ Imagen: `%s`", icon, name, status, image)
		
		if project != "" {
			text += fmt.Sprintf("\n   ├ Proyecto: `%s`", project)
		}
		
		text += fmt.Sprintf("\n   ├ CPU: `%s`\n   └ RAM: `%s`", stat.CPU, stat.Mem)
		
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
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
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

func handleRunning(chatID int64) {
	loadingID := sendLoading(chatID, "Cargando contenedores...")
	defer deleteMsg(chatID, loadingID)
	out, err := runCmd("docker", "ps", "-a", "--format", "{{.Names}}|{{.Status}}|{{.Image}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		name, status, image := parts[0], parts[1], parts[2]
		icon := getIcon(name)
		statusIcon := "🔴"
		if strings.Contains(status, "Up") {
			statusIcon = "🟢"
		}
		
		// Get compose project
		inspectOut, _ := runCmd("docker", "inspect", name)
		var project string
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Config"].(map[string]interface{})["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		
		text := fmt.Sprintf("%s %s *%s*\n   ├ Estado: `%s`\n   └ Imagen: `%s`", statusIcon, icon, name, status, image)
		if project != "" {
			text = fmt.Sprintf("%s %s *%s*\n   ├ Estado: `%s`\n   ├ Imagen: `%s`\n   └ Proyecto: `%s`", statusIcon, icon, name, status, image, project)
		}
		
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		
		var keyboard tgbotapi.InlineKeyboardMarkup
		if strings.Contains(status, "Up") {
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
	out, err := runCmd("docker", "ps", "-a", "--format", "{{.Names}}|{{.Status}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		parts1 := strings.SplitN(lines[i], "|", 2)
		if len(parts1) < 2 {
			continue
		}
		name1, status1 := parts1[0], parts1[1]
		dot1 := "🔴"
		if strings.Contains(status1, "Up") {
			dot1 = "🟢"
		} else if strings.Contains(status1, "Paused") {
			dot1 = "🟡"
		}
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(dot1+" "+getIcon(name1)+" "+name1, "container_menu:"+name1),
		}
		if i+1 < len(lines) {
			parts2 := strings.SplitN(lines[i+1], "|", 2)
			if len(parts2) >= 2 {
				name2, status2 := parts2[0], parts2[1]
				dot2 := "🔴"
				if strings.Contains(status2, "Up") {
					dot2 = "🟢"
				} else if strings.Contains(status2, "Paused") {
					dot2 = "🟡"
				}
				row = append(row, tgbotapi.NewInlineKeyboardButtonData(dot2+" "+getIcon(name2)+" "+name2, "container_menu:"+name2))
			}
		}
		keyboard = append(keyboard, row)
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🐳 *Contenedores* (%d)", len(lines)))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleGrid(chatID int64, title, action string, allContainers bool) {
	args := []string{"ps", "--format", "{{.Names}}|{{.Status}}"}
	if allContainers {
		args = append(args, "-a")
	}
	out, err := runCmd("docker", args...)
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		parts1 := strings.SplitN(lines[i], "|", 2)
		if len(parts1) < 2 {
			continue
		}
		name1, status1 := parts1[0], parts1[1]
		dot1 := "🔴"
		if strings.Contains(status1, "Up") {
			dot1 = "🟢"
		} else if strings.Contains(status1, "Paused") {
			dot1 = "🟡"
		}
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(dot1+" "+getIcon(name1)+" "+name1, action+":"+name1),
		}
		if i+1 < len(lines) {
			parts2 := strings.SplitN(lines[i+1], "|", 2)
			if len(parts2) >= 2 {
				name2, status2 := parts2[0], parts2[1]
				dot2 := "🔴"
				if strings.Contains(status2, "Up") {
					dot2 = "🟢"
				} else if strings.Contains(status2, "Paused") {
					dot2 = "🟡"
				}
				row = append(row, tgbotapi.NewInlineKeyboardButtonData(dot2+" "+getIcon(name2)+" "+name2, action+":"+name2))
			}
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
	
	// Handle close button
	if query.Data == "close" {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID)
		bot.Send(deleteMsg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	}
	
	parts := strings.SplitN(query.Data, ":", 2)
	if len(parts) != 2 {
		log.Printf("Invalid callback data: %s", query.Data)
		return
	}
	action, target := parts[0], parts[1]
	
	log.Printf("Callback: action=%s, target=%s, chatID=%d", action, target, chatID)
	
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
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "create_type":
		userID := query.From.ID
		if target == "run" {
			go handleCreateRun(chatID, userID)
		} else if target == "compose" {
			go handleCreateCompose(chatID, userID)
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "create_exec":
		out, err := runCmd("sh", "-c", target)
		if err != nil {
			out = "❌ Error: " + err.Error()
		} else {
			out = "✅ Contenedor creado exitosamente\n```\n" + out + "\n```"
		}
		msg := tgbotapi.NewMessage(chatID, out)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "inspect_containers":
		go handleList(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "inspect_images":
		out, _ := runCmd("docker", "images", "--format", "{{.Repository}}:{{.Tag}}|{{.ID}}")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		var keyboard [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(lines); i += 2 {
			parts := strings.Split(lines[i], "|")
			if len(parts) < 2 {
				continue
			}
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("🖼️ "+parts[0], "inspect_img:"+parts[1]),
			}
			if i+1 < len(lines) {
				parts2 := strings.Split(lines[i+1], "|")
				if len(parts2) >= 2 {
					row = append(row, tgbotapi.NewInlineKeyboardButtonData("🖼️ "+parts2[0], "inspect_img:"+parts2[1]))
				}
			}
			keyboard = append(keyboard, row)
		}
		msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar imagen*")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "inspect_volumes":
		out, _ := runCmd("docker", "volume", "ls", "--format", "{{.Name}}")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		var keyboard [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(lines); i += 2 {
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("💾 "+lines[i], "inspect_vol:"+lines[i]),
			}
			if i+1 < len(lines) {
				row = append(row, tgbotapi.NewInlineKeyboardButtonData("💾 "+lines[i+1], "inspect_vol:"+lines[i+1]))
			}
			keyboard = append(keyboard, row)
		}
		msg := tgbotapi.NewMessage(chatID, "🔍 *Inspeccionar volumen*")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "inspect_networks":
		out, _ := runCmd("docker", "network", "ls", "--format", "{{.Name}}")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		var keyboard [][]tgbotapi.InlineKeyboardButton
		for i := 0; i < len(lines); i += 2 {
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("🌐 "+lines[i], "inspect_net:"+lines[i]),
			}
			if i+1 < len(lines) {
				row = append(row, tgbotapi.NewInlineKeyboardButtonData("🌐 "+lines[i+1], "inspect_net:"+lines[i+1]))
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
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "back_main":
		go handleStart(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "exec_menu_back":
		go handleExecMenu(chatID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "compose_menu":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("▶️ Up", "compose_up:"+target),
				tgbotapi.NewInlineKeyboardButtonData("⏸️ Down", "compose_down:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "compose_restart:"+target),
				tgbotapi.NewInlineKeyboardButtonData("📋 PS", "compose_ps:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔄 Pull", "compose_pull:"+target),
				tgbotapi.NewInlineKeyboardButtonData("📄 Ver compose.yml", "compose_view:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "cmd:compose"),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📁 *Proyecto: %s*\n¿Qué deseas hacer?", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "compose_up":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Iniciando proyecto *%s*...", target))
		workDir := getComposeWorkDir(target)
		if workDir != "" {
			out, err = runCmd("docker", "compose", "--project-directory", workDir, "-p", target, "up", "-d")
		} else {
			out, err = runCmd("docker", "compose", "-p", target, "up", "-d")
		}
		if err == nil {
			out = fmt.Sprintf("✅ Proyecto *%s* iniciado\n```\n%s\n```", target, out)
		}
	case "compose_down":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Deteniendo proyecto *%s*...", target))
		workDir := getComposeWorkDir(target)
		if workDir != "" {
			out, err = runCmd("docker", "compose", "--project-directory", workDir, "-p", target, "down")
		} else {
			out, err = runCmd("docker", "compose", "-p", target, "down")
		}
		if err == nil {
			out = fmt.Sprintf("✅ Proyecto *%s* detenido\n```\n%s\n```", target, out)
		}
	case "compose_restart":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Reiniciando proyecto *%s*...", target))
		workDir := getComposeWorkDir(target)
		if workDir != "" {
			out, err = runCmd("docker", "compose", "--project-directory", workDir, "-p", target, "restart")
		} else {
			out, err = runCmd("docker", "compose", "-p", target, "restart")
		}
		if err == nil {
			out = fmt.Sprintf("✅ Proyecto *%s* reiniciado", target)
		}
	case "compose_ps":
		// Get containers from this project
		containersOut, _ := runCmd("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+target, "--format", "{{.Names}}|{{.Status}}")

		if containersOut == "" {
			out = fmt.Sprintf("No hay contenedores en el proyecto *%s*", target)
		} else {
			lines := strings.Split(strings.TrimSpace(containersOut), "\n")
			var kbRows [][]tgbotapi.InlineKeyboardButton
			for i := 0; i < len(lines); i += 2 {
				parts := strings.SplitN(lines[i], "|", 2)
				if len(parts) < 2 {
					continue
				}
				name1, status1 := parts[0], parts[1]
				dot1 := "🔴"
				if strings.Contains(status1, "Up") {
					dot1 = "🟢"
				} else if strings.Contains(status1, "Paused") {
					dot1 = "🟡"
				}
				row := []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData(dot1+" "+getIcon(name1)+" "+name1, "container_menu:"+name1),
				}
				if i+1 < len(lines) {
					parts2 := strings.SplitN(lines[i+1], "|", 2)
					if len(parts2) >= 2 {
						name2, status2 := parts2[0], parts2[1]
						dot2 := "🔴"
						if strings.Contains(status2, "Up") {
							dot2 = "🟢"
						} else if strings.Contains(status2, "Paused") {
							dot2 = "🟡"
						}
						row = append(row, tgbotapi.NewInlineKeyboardButtonData(dot2+" "+getIcon(name2)+" "+name2, "container_menu:"+name2))
					}
				}
				kbRows = append(kbRows, row)
			}
			kbRows = append(kbRows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬅️ Atrás", "compose_menu:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			))
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📁 *Proyecto: %s* (%d contenedores)", target, len(lines)))
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(kbRows...)
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
	case "compose_pull":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Descargando imágenes de *%s*...", target))
		workDir := getComposeWorkDir(target)
		if workDir != "" {
			out, err = runCmd("docker", "compose", "--project-directory", workDir, "-p", target, "pull")
		} else {
			out, err = runCmd("docker", "compose", "-p", target, "pull")
		}
		if err == nil {
			out = fmt.Sprintf("✅ Imágenes de *%s* actualizadas\n```\n%s\n```", target, out)
		}
	case "prune":
		switch target {
		case "images":
			out, err = runCmd("docker", "image", "prune", "-af")
			if err == nil {
				out = "✅ Imágenes no usadas eliminadas\n```\n" + out + "\n```"
			}
		case "volumes":
			out, err = runCmd("docker", "volume", "prune", "-f")
			if err == nil {
				out = "✅ Volúmenes no usados eliminados\n```\n" + out + "\n```"
			}
		case "networks":
			out, err = runCmd("docker", "network", "prune", "-f")
			if err == nil {
				out = "✅ Redes no usadas eliminadas\n```\n" + out + "\n```"
			}
		case "all":
			out, err = runCmd("docker", "system", "prune", "-af", "--volumes")
			if err == nil {
				out = "✅ Sistema limpiado\n```\n" + out + "\n```"
			}
		}
	case "exec_menu":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📋 ps aux", "exec_cmd:"+target+":ps aux"),
				tgbotapi.NewInlineKeyboardButtonData("📁 ls -la /", "exec_cmd:"+target+":ls -la /"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🌐 netstat", "exec_cmd:"+target+":netstat -tulpn"),
				tgbotapi.NewInlineKeyboardButtonData("💾 df -h", "exec_cmd:"+target+":df -h"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🧠 free -m", "exec_cmd:"+target+":free -m"),
				tgbotapi.NewInlineKeyboardButtonData("🔧 env", "exec_cmd:"+target+":env"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🐧 OS", "exec_cmd:"+target+":cat /etc/os-release"),
				tgbotapi.NewInlineKeyboardButtonData("⏱️ uptime", "exec_cmd:"+target+":uptime"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⌨️ Comando manual", "exec_manual:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚙️ *Ejecutar en: %s*\nSelecciona un comando:", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "exec_manual":
		userID := query.From.ID
		userState[userID] = "waiting_exec_cmd:" + target
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⌨️ *Comando manual en: %s*\nEscribe el comando a ejecutar:", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "exec_sh":
		out = fmt.Sprintf("💡 Para ejecutar shell interactivo:\n```\ndocker exec -it %s /bin/sh\n```", target)
	case "exec_bash":
		out = fmt.Sprintf("💡 Para ejecutar bash interactivo:\n```\ndocker exec -it %s /bin/bash\n```", target)
	case "exec_cmd":
		parts := strings.SplitN(target, ":", 2)
		if len(parts) == 2 {
			container, cmd := parts[0], parts[1]
			cmdParts := strings.Fields(cmd)
			out, err = runCmd("docker", append([]string{"exec", container}, cmdParts...)...)
			if err == nil {
				out = fmt.Sprintf("⚙️ *Resultado de: %s*\n```\n%s\n```", cmd, out)
			}
		}
	case "logs":
		out, err = runCmd("docker", "logs", "--tail", "30", target)
		if err == nil {
			// Highlight errors and warnings
			lines := strings.Split(out, "\n")
			highlighted := []string{}
			for _, line := range lines {
				lineLower := strings.ToLower(line)
				if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "fatal") || strings.Contains(lineLower, "exception") {
					highlighted = append(highlighted, "🔴 "+line)
				} else if strings.Contains(lineLower, "warn") {
					highlighted = append(highlighted, "🟡 "+line)
				} else {
					highlighted = append(highlighted, line)
				}
			}
			out = fmt.Sprintf("📊 *Logs de %s*\n```\n%s\n```", target, strings.Join(highlighted, "\n"))
			
			// Add filter buttons
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
			container, filter := parts[0], parts[1]
			logsOut, _ := runCmd("docker", "logs", "--tail", "100", container)
			lines := strings.Split(logsOut, "\n")
			filtered := []string{}
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), filter) {
					filtered = append(filtered, line)
				}
			}
			if len(filtered) > 0 {
				out = fmt.Sprintf("📊 *Logs filtrados (%s) de %s*\n```\n%s\n```", filter, container, strings.Join(filtered, "\n"))
			} else {
				out = fmt.Sprintf("No se encontraron logs con '%s'", filter)
			}
		}
	case "logs_more":
		out, err = runCmd("docker", "logs", "--tail", "100", target)
	case "logfile":
		// Get all logs
		logsOut, err := runCmd("docker", "logs", "--tail", "1000", target)
		if err != nil {
			out = "❌ Error obteniendo logs: " + err.Error()
		} else {
			// Create temp file
			filename := fmt.Sprintf("/tmp/%s_%d.log", target, time.Now().Unix())
			err := os.WriteFile(filename, []byte(logsOut), 0644)
			if err != nil {
				out = "❌ Error creando archivo: " + err.Error()
			} else {
				// Send file
				doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filename))
				doc.Caption = fmt.Sprintf("📋 Logs de *%s*", target)
				doc.ParseMode = "Markdown"
				bot.Send(doc)
				
				// Delete temp file
				os.Remove(filename)
				
				bot.Request(tgbotapi.NewCallback(query.ID, "✅ Archivo generado"))
				return
			}
		}
		if err == nil {
			out = fmt.Sprintf("📊 *Logs completos de %s*\n```\n%s\n```", target, out)
		}
	case "pause":
		out, err = runCmd("docker", "pause", target)
		if err == nil {
			out = fmt.Sprintf("⏸️ *%s* pausado", target)
		}
	case "unpause":
		out, err = runCmd("docker", "unpause", target)
		if err == nil {
			out = fmt.Sprintf("▶️ *%s* reanudado", target)
		}
	case "env":
		envOut, _ := runCmd("docker", "inspect", "--format", "{{range .Config.Env}}{{println .}}{{end}}", target)
		if envOut != "" {
			if len(envOut) > 3800 {
				envOut = envOut[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔧 *Variables de entorno de %s*\n```\n%s\n```", target, envOut))
			msg.ParseMode = "Markdown"
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		} else {
			out = fmt.Sprintf("No hay variables de entorno en *%s*", target)
		}
	case "fav_action":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+target),
				tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+target),
				tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Quitar de favoritos", "removefav:"+target),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⭐ *%s*\n¿Qué deseas hacer?", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "removefav":
		userID := query.From.ID
		newFavs := []string{}
		for _, fav := range favorites[userID] {
			if fav != target {
				newFavs = append(newFavs, fav)
			}
		}
		favorites[userID] = newFavs
		out = fmt.Sprintf("✅ *%s* eliminado de favoritos", target)
	case "togglefav":
		userID := query.From.ID
		found := false
		newFavs := []string{}
		
		// Check if already in favorites
		for _, fav := range favorites[userID] {
			if fav == target {
				found = true
			} else {
				newFavs = append(newFavs, fav)
			}
		}
		
		if found {
			// Remove from favorites
			favorites[userID] = newFavs
			out = fmt.Sprintf("❌ *%s* quitado de favoritos", target)
		} else {
			// Add to favorites
			favorites[userID] = append(favorites[userID], target)
			out = fmt.Sprintf("✅ *%s* agregado a favoritos", target)
		}
		
		// Refresh the menu
		go handleAddFavoriteMenu(chatID, userID)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "restart":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Reiniciando *%s*...", target))
		out, err = runCmd("docker", "restart", target)
		if err == nil {
			out = fmt.Sprintf("✅ *%s* reiniciado", target)
		}
	case "stop":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Deteniendo *%s*...", target))
		out, err = runCmd("docker", "stop", target)
		if err == nil {
			out = fmt.Sprintf("✅ *%s* detenido", target)
		}
	case "start":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Iniciando *%s*...", target))
		out, err = runCmd("docker", "start", target)
		if err == nil {
			// Wait a bit for container to start
			time.Sleep(2 * time.Second)
			
			// Check status
			statusOut, _ := runCmd("docker", "inspect", "--format", "{{.State.Status}}", target)
			status := strings.TrimSpace(statusOut)
			
			if status == "running" {
				// Get stats
				statsOut, _ := runCmd("docker", "stats", "--no-stream", "--format", "{{.CPUPerc}}|{{.MemUsage}}", target)
				parts := strings.Split(strings.TrimSpace(statsOut), "|")
				cpu, mem := "N/A", "N/A"
				if len(parts) >= 2 {
					cpu, mem = parts[0], parts[1]
				}
				
				icon := getIcon(target)
				out = fmt.Sprintf("✅ %s *%s* iniciado\n\n🟢 Estado: `running`\n📊 CPU: `%s`\n💾 RAM: `%s`", icon, target, cpu, mem)
			} else {
				// Get logs to see why it failed
				logsOut, _ := runCmd("docker", "logs", "--tail", "20", target)
				icon := getIcon(target)
				out = fmt.Sprintf("⚠️ %s *%s* no inició correctamente\n\n🔴 Estado: `%s`\n\n📋 Últimos logs:\n```\n%s\n```", icon, target, status, logsOut)
			}
		}
	case "remove":
		out, err = runCmd("docker", "rm", "-f", target)
		if err == nil {
			out = fmt.Sprintf("✅ *%s* eliminado", target)
		}
	case "rmi":
		out, err = runCmd("docker", "rmi", target)
		if err == nil {
			out = "✅ Imagen eliminada"
		}
	case "container_menu":
		// Get container status
		statusOut, _ := runCmd("docker", "inspect", "--format", "{{.State.Status}}", target)
		status := strings.TrimSpace(statusOut)
		icon := getIcon(target)
		statusIcon := "🔴"
		if status == "running" {
			statusIcon = "🟢"
		} else if status == "paused" {
			statusIcon = "🟡"
		}

		var rows [][]tgbotapi.InlineKeyboardButton
		if status == "running" {
			rows = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📊 Logs", "logs:"+target),
					tgbotapi.NewInlineKeyboardButtonData("💾 Logfile", "logfile:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("📈 Stats", "container_stats:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔌 Puertos", "ports:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 Restart", "restart:"+target),
					tgbotapi.NewInlineKeyboardButtonData("⏸️ Stop", "stop:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("⚙️ Exec", "exec_menu:"+target),
					tgbotapi.NewInlineKeyboardButtonData("🔧 Env", "env:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔍 Inspect", "inspect:"+target),
					tgbotapi.NewInlineKeyboardButtonData("⏸️ Pause", "pause:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✏️ Rename", "rename_prompt:"+target),
					tgbotapi.NewInlineKeyboardButtonData("📋 Copy", "copy_prompt:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑️ Remove", "remove_confirm:"+target),
				),
			}
		} else if status == "paused" {
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
					tgbotapi.NewInlineKeyboardButtonData("✏️ Rename", "rename_prompt:"+target),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑️ Remove", "remove_confirm:"+target),
				),
			}
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		))
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s %s *%s*\nEstado: `%s`\n¿Qué deseas hacer?", statusIcon, icon, target, status))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "container_stats":
		statsOut, _ := runCmd("docker", "stats", "--no-stream", "--format", "{{.CPUPerc}}|{{.MemUsage}}|{{.NetIO}}|{{.BlockIO}}", target)
		parts := strings.Split(strings.TrimSpace(statsOut), "|")
		cpu, mem, net, blk := "N/A", "N/A", "N/A", "N/A"
		if len(parts) >= 4 {
			cpu, mem, net, blk = parts[0], parts[1], parts[2], parts[3]
		}
		icon := getIcon(target)
		text := fmt.Sprintf("📈 *Stats: %s %s*\n\n🔥 CPU: `%s`\n💾 RAM: `%s`\n🌐 Net I/O: `%s`\n💿 Block I/O: `%s`", icon, target, cpu, mem, net, blk)
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "container_stats:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "ports":
		portsOut, _ := runCmd("docker", "inspect", "--format", "{{range $p, $b := .NetworkSettings.Ports}}{{$p}} -> {{range $b}}{{.HostIP}}:{{.HostPort}}{{end}}\n{{end}}", target)
		portsOut = strings.TrimSpace(portsOut)
		if portsOut == "" {
			portsOut = "Sin puertos expuestos"
		}
		sendMessageWithClose(chatID, fmt.Sprintf("🔌 *Puertos de %s*\n```\n%s\n```", target, portsOut))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "top":
		topOut, err := runCmd("docker", "top", target)
		if err != nil {
			sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		} else {
			if len(topOut) > 3800 {
				topOut = topOut[:3800] + "\n...(truncado)"
			}
			sendMessageWithClose(chatID, fmt.Sprintf("🔝 *Procesos en %s*\n```\n%s\n```", target, topOut))
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "uptime":
		startedOut, _ := runCmd("docker", "inspect", "--format", "{{.State.StartedAt}}", target)
		startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startedOut))
		var uptimeText string
		if err == nil {
			d := time.Since(startedAt)
			days := int(d.Hours()) / 24
			hours := int(d.Hours()) % 24
			mins := int(d.Minutes()) % 60
			uptimeText = fmt.Sprintf("%dd %dh %dm", days, hours, mins)
		} else {
			uptimeText = "N/A"
		}
		sendMessageWithClose(chatID, fmt.Sprintf("⏱️ *Uptime de %s*\n`%s`", target, uptimeText))
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "rename_prompt":
		userState[query.From.ID] = "waiting_rename:" + target
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✏️ *Renombrar: %s*\nEscribe el nuevo nombre:", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close")),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "copy_prompt":
		userState[query.From.ID] = "waiting_copy:" + target
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📋 *Copiar: %s*\nEscribe el nombre del nuevo contenedor:", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close")),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "remove_confirm":
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ *¿Eliminar %s?*\nEsta acción no se puede deshacer.", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Sí, eliminar", "remove:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
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
	case "prune_confirm":
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ *¿Ejecutar prune de %s?*\nSe eliminarán recursos no usados.", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Confirmar", "prune:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "rmvol_confirm":
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ *¿Eliminar volumen %s?*\nSe perderán todos los datos.", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Sí, eliminar", "rmvol:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "rmnet_confirm":
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ *¿Eliminar red %s?*", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Sí, eliminar", "rmnet:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "rmi_confirm":
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ *¿Eliminar imagen?*\n`%s`", target))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Sí, eliminar", "rmi:"+target),
				tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "close"),
			),
		)
		bot.Send(msg)
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "compose_view":
		workDir := getComposeWorkDir(target)
		if workDir == "" {
			sendMessageWithClose(chatID, "❌ No se encontró el directorio del proyecto")
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
		// Try common compose file names
		var content string
		for _, fname := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
			out, err := runCmd("cat", workDir+"/"+fname)
			if err == nil {
				content = out
				break
			}
		}
		if content == "" {
			sendMessageWithClose(chatID, "❌ No se encontró el archivo compose en `"+workDir+"`")
		} else {
			if len(content) > 3800 {
				content = content[:3800] + "\n...(truncado)"
			}
			sendMessageWithClose(chatID, fmt.Sprintf("📄 *compose.yml — %s*\n```yaml\n%s\n```", target, content))
		}
		bot.Request(tgbotapi.NewCallback(query.ID, ""))
		return
	case "inspect":
		out, err = runCmd("docker", "inspect", target)
		if err == nil {
			// Send as plain text to avoid markdown issues
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect %s*\n```\n%s\n```", target, out))
			msg.ParseMode = "Markdown"
			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
				),
			)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
	case "inspect_img":
		out, err = runCmd("docker", "inspect", target)
		if err == nil {
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect imagen*\n```\n%s\n```", out))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
	case "inspect_vol":
		out, err = runCmd("docker", "volume", "inspect", target)
		if err == nil {
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect volumen*\n```\n%s\n```", out))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
	case "rmvol":
		out, err = runCmd("docker", "volume", "rm", target)
		if err == nil {
			out = "✅ Volumen eliminado"
		}
	case "update_compose":
		// Get all containers in this compose project and recreate each one
		psOut, _ := runCmd("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+target, "--format", "{{.Names}}")
		containers := strings.Split(strings.TrimSpace(psOut), "\n")
		if len(containers) == 0 || containers[0] == "" {
			out = "❌ No se encontraron contenedores en el proyecto `" + target + "`"
			break
		}
		var results []string
		for _, cname := range containers {
			if cname == "" {
				continue
			}
			if err2 := recreateContainer(cname); err2 != nil {
				results = append(results, "❌ "+cname+": "+err2.Error())
			} else {
				results = append(results, "✅ "+cname)
			}
		}
		out = fmt.Sprintf("🔄 *Proyecto %s actualizado*\n%s", target, strings.Join(results, "\n"))
	case "pull_image":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Descargando imagen `%s`...", target))
		if _, err = runCmd("docker", "pull", target); err == nil {
			out = fmt.Sprintf("✅ Imagen `%s` actualizada\nEl contenedor seguirá usando la imagen anterior hasta que lo recrees.", target)
		}
	case "pull_recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Actualizando y recreando *%s*...", target))
		// Get current image and digest before pull
		inspectOut, inspectErr := runCmd("docker", "inspect", target)
		if inspectErr != nil {
			out = "❌ No se pudo inspeccionar el contenedor"
			break
		}
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) != nil || len(inspectData) == 0 {
			out = "❌ No se pudo parsear la inspección"
			break
		}
		config, _ := inspectData[0]["Config"].(map[string]interface{})
		image, _ := config["Image"].(string)
		if image == "" {
			out = "❌ No se pudo obtener la imagen del contenedor"
			break
		}
		oldDigest, _ := runCmd("docker", "inspect", "--format", "{{index .RepoDigests 0}}", image)
		oldDigest = strings.TrimSpace(oldDigest)

		if _, pullErr := runCmd("docker", "pull", image); pullErr != nil {
			out = "❌ Error al hacer pull: " + pullErr.Error()
			break
		}

		newDigest, _ := runCmd("docker", "inspect", "--format", "{{index .RepoDigests 0}}", image)
		newDigest = strings.TrimSpace(newDigest)

		if oldDigest != "" && oldDigest == newDigest {
			out = fmt.Sprintf("ℹ️ La imagen `%s` ya estaba actualizada. No se recreó el contenedor.", image)
			break
		}

		if recErr := recreateContainer(target); recErr != nil {
			out = "✅ Pull exitoso, pero error al recrear: " + recErr.Error()
		} else {
			out = fmt.Sprintf("✅ *%s* actualizado y recreado con la nueva imagen `%s`", target, image)
		}
	case "recreate":
		editToLoading(chatID, query.Message.MessageID, fmt.Sprintf("Recreando *%s*...", target))
		if err2 := recreateContainer(target); err2 != nil {
			out = "❌ Error: " + err2.Error()
		} else {
			out = fmt.Sprintf("✅ *%s* recreado con la nueva imagen", target)
		}
	case "inspect_net":
		out, err = runCmd("docker", "network", "inspect", target)
		if err == nil {
			if len(out) > 3800 {
				out = out[:3800] + "\n...\n(truncado)"
			}
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 *Inspect red*\n```\n%s\n```", out))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			bot.Request(tgbotapi.NewCallback(query.ID, ""))
			return
		}
	case "rmnet":
		out, err = runCmd("docker", "network", "rm", target)
		if err == nil {
			out = "✅ Red eliminada"
		}
	}
	
	if err != nil {
		out = "❌ Error: " + err.Error()
		log.Printf("Error in callback %s: %v", action, err)
	}

	log.Printf("Sending response: %d chars", len(out))

	// Actions that called editToLoading: edit the same message with the result
	loadingActions := map[string]bool{
		"restart": true, "stop": true, "start": true,
		"recreate": true, "pull_image": true, "pull_recreate": true,
		"compose_up": true, "compose_down": true, "compose_restart": true, "compose_pull": true,
	}
	if loadingActions[action] {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, out)
		edit.ParseMode = "Markdown"
		edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close")},
		}}
		bot.Send(edit)
	} else {
		msg := tgbotapi.NewMessage(chatID, out)
		msg.ParseMode = "Markdown"
		_, sendErr := bot.Send(msg)
		if sendErr != nil {
			log.Printf("Error sending message: %v", sendErr)
		}
	}
	bot.Request(tgbotapi.NewCallback(query.ID, ""))
}

func monitorEvents() {
	for {
		cmd := exec.Command("docker", "events", "--format", "{{json .}}")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Println("Error monitoring events:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		
		if err := cmd.Start(); err != nil {
			log.Println("Error starting events:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		
		decoder := json.NewDecoder(stdout)
		for {
			var event map[string]interface{}
			if err := decoder.Decode(&event); err != nil {
				break
			}

			if notifyChatID == 0 {
				continue
			}

			action, _ := event["Action"].(string)
			actor, _ := event["Actor"].(map[string]interface{})
			attrs, _ := actor["Attributes"].(map[string]interface{})
			name, _ := attrs["name"].(string)
			image, _ := attrs["image"].(string)
			exitCode, _ := attrs["exitCode"].(string)

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

			switch action {
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
				lastLogs, _ := runCmd("docker", "logs", "--tail", "5", name)
				lastLogs = strings.TrimSpace(lastLogs)
				logsSection := ""
				if lastLogs != "" {
					if len(lastLogs) > 500 {
						lastLogs = lastLogs[len(lastLogs)-500:]
					}
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
		}
		
		cmd.Wait()
		time.Sleep(5 * time.Second)
	}
}

func monitorResourceAlerts() {
	alertedContainers := make(map[string]time.Time)
	// containers that exceeded threshold on first check, pending confirmation
	pendingAlerts := make(map[string]bool)

	collectStats := func() map[string][2]float64 {
		result := make(map[string][2]float64)
		statsOut, err := runCmd("docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemPerc}}")
		if err != nil {
			return result
		}
		for _, line := range strings.Split(statsOut, "\n") {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) < 3 {
				continue
			}
			name := parts[0]
			var cpu, mem float64
			fmt.Sscanf(strings.TrimSuffix(parts[1], "%"), "%f", &cpu)
			fmt.Sscanf(strings.TrimSuffix(parts[2], "%"), "%f", &mem)
			result[name] = [2]float64{cpu, mem}
		}
		return result
	}

	for {
		time.Sleep(5 * time.Minute)

		if notifyChatID == 0 {
			continue
		}

		first := collectStats()

		// Identify containers above threshold
		candidates := make(map[string]bool)
		for name, vals := range first {
			if vals[0] > 90 || vals[1] > 90 {
				candidates[name] = true
			}
		}

		// Only confirm containers that were already pending from last cycle
		toAlert := make(map[string][2]float64)
		for name := range candidates {
			if pendingAlerts[name] {
				toAlert[name] = first[name]
			}
		}

		// Update pending: only containers that are candidates this cycle
		pendingAlerts = candidates

		for name, vals := range toAlert {
			if lastAlert, exists := alertedContainers[name]; exists {
				if time.Since(lastAlert) < 30*time.Minute {
					continue
				}
			}

			cpu, mem := vals[0], vals[1]
			icon := getIcon(name)
			var alertType string
			if cpu > 90 && mem > 90 {
				alertType = fmt.Sprintf("🔥 CPU: %.1f%% | RAM: %.1f%%", cpu, mem)
			} else if cpu > 90 {
				alertType = fmt.Sprintf("🔥 CPU: %.1f%%", cpu)
			} else {
				alertType = fmt.Sprintf("💾 RAM: %.1f%%", mem)
			}

			msg := fmt.Sprintf("⚠️ *Alerta de recursos*\n\n%s *%s*\n%s", icon, name, alertType)
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

		containersOut, _ := runCmd("docker", "ps", "-a", "-q")
		runningOut, _ := runCmd("docker", "ps", "-q")
		imagesOut, _ := runCmd("docker", "images", "-q")

		containerCount := len(strings.Split(strings.TrimSpace(containersOut), "\n"))
		runningCount := len(strings.Split(strings.TrimSpace(runningOut), "\n"))
		imageCount := len(strings.Split(strings.TrimSpace(imagesOut), "\n"))

		stoppedOut, _ := runCmd("docker", "ps", "-a", "--filter", "status=exited", "-q")
		unhealthyOut, _ := runCmd("docker", "ps", "--filter", "health=unhealthy", "--format", "{{.Names}}")

		stoppedCount := 0
		if strings.TrimSpace(stoppedOut) != "" {
			stoppedCount = len(strings.Split(strings.TrimSpace(stoppedOut), "\n"))
		}
		unhealthyCount := 0
		if strings.TrimSpace(unhealthyOut) != "" {
			unhealthyCount = len(strings.Split(strings.TrimSpace(unhealthyOut), "\n"))
		}

		status := "✅ Todo bien"
		if stoppedCount > 0 || unhealthyCount > 0 {
			status = "⚠️ Requiere atención"
		}

		report := fmt.Sprintf("📊 *Reporte Diario - %s*\n\n%s\n\n🐳 *Resumen:*\n  • Contenedores: %d (%d corriendo)\n  • Imágenes: %d\n  • Detenidos: %d\n  • Unhealthy: %d",
			now.Format("02/01/2006"), status, containerCount, runningCount, imageCount, stoppedCount, unhealthyCount)

		m := tgbotapi.NewMessage(notifyChatID, report)
		m.ParseMode = "Markdown"
		bot.Send(m)

		// Weekly report every 7 days
		weeklyCount++
		if weeklyCount >= 7 {
			weeklyCount = 0
			dfOut, _ := runCmd("df", "-h", "/")
			diskInfo := "N/A"
			diskLines := strings.Split(dfOut, "\n")
			if len(diskLines) > 1 {
				fields := strings.Fields(diskLines[1])
				if len(fields) >= 5 {
					diskInfo = fmt.Sprintf("%s / %s (%s)", fields[2], fields[1], fields[4])
				}
			}
			volumesOut, _ := runCmd("docker", "volume", "ls", "-q")
			volumeCount := len(strings.Split(strings.TrimSpace(volumesOut), "\n"))
			networksOut, _ := runCmd("docker", "network", "ls", "-q")
			networkCount := len(strings.Split(strings.TrimSpace(networksOut), "\n"))

			weekly := fmt.Sprintf("📅 *Reporte Semanal - %s*\n\n%s\n\n🐳 *Docker:*\n  • Contenedores: %d (%d corriendo)\n  • Imágenes: %d\n  • Volúmenes: %d\n  • Redes: %d\n\n🖥️ *Sistema:*\n  • Disco: %s",
				now.Format("02/01/2006"), status, containerCount, runningCount, imageCount, volumeCount, networkCount, diskInfo)
			wm := tgbotapi.NewMessage(notifyChatID, weekly)
			wm.ParseMode = "Markdown"
			bot.Send(wm)
		}
	}
}

func checkUpdates() {
	// First check after 5 minutes, then every 6 hours
	time.Sleep(5 * time.Minute)
	for {
		if notifyChatID != 0 {
			runImageUpdateCheck()
		}
		time.Sleep(6 * time.Hour)
	}
}

func runImageUpdateCheck() int {
	out, err := runCmd("docker", "ps", "-a", "--format", "{{.Names}}|{{.Image}}")
	if err != nil {
		return 0
	}

	// Build map: image -> list of (containerName, composeProject)
	type containerInfo struct {
		name    string
		project string
	}
	imageMap := make(map[string][]containerInfo)

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		name, image := parts[0], parts[1]

		// Get compose project label
		var project string
		inspectOut, _ := runCmd("docker", "inspect", name)
		var inspectData []map[string]interface{}
		if json.Unmarshal([]byte(inspectOut), &inspectData) == nil && len(inspectData) > 0 {
			if labels, ok := inspectData[0]["Config"].(map[string]interface{})["Labels"].(map[string]interface{}); ok {
				if p, ok := labels["com.docker.compose.project"].(string); ok {
					project = p
				}
			}
		}
		imageMap[image] = append(imageMap[image], containerInfo{name, project})
	}

	// Check each unique image once
	found := 0
	for image, containers := range imageMap {
		localDigest, _ := runCmd("docker", "inspect", "--format", "{{index .RepoDigests 0}}", image)
		localDigest = strings.TrimSpace(localDigest)

		runCmd("docker", "pull", image)

		newDigest, _ := runCmd("docker", "inspect", "--format", "{{index .RepoDigests 0}}", image)
		newDigest = strings.TrimSpace(newDigest)

		if localDigest == "" || localDigest == newDigest {
			continue
		}
		found++

		// Collect unique compose projects for this image
		projectSet := make(map[string]bool)
		for _, c := range containers {
			if c.project != "" {
				projectSet[c.project] = true
			}
		}

		// Build notification
		icon := getIcon(containers[0].name)
		names := make([]string, 0, len(containers))
		for _, c := range containers {
			names = append(names, c.name)
		}
		msgText := fmt.Sprintf("🆕 %s *Nueva versión disponible*\nImagen: `%s`\nContenedor(es): `%s`",
			icon, image, strings.Join(names, "`, `"))

		if len(projectSet) > 0 {
			projects := make([]string, 0, len(projectSet))
			for p := range projectSet {
				projects = append(projects, p)
			}
			msgText += fmt.Sprintf("\nProyecto(s): `%s`", strings.Join(projects, "`, `"))
		}

		m := tgbotapi.NewMessage(notifyChatID, msgText)
		m.ParseMode = "Markdown"

		// One row per container: [pull only] [recreate] [pull & recreate]
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, c := range containers {
			recreateLabel := "🔄 Recrear: " + c.name
			if c.project != "" {
				recreateLabel = "🔄 Recrear: " + c.name + " (" + c.project + ")"
			}
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬇️ Solo pull: "+image, "pull_image:"+image),
				tgbotapi.NewInlineKeyboardButtonData(recreateLabel, "recreate:"+c.name),
			))
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⬆️ Pull & Recrear: "+c.name, "pull_recreate:"+c.name),
			))
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

	log.Printf("Bot iniciado: @%s", bot.Self.UserName)

	// Load allowed users from env
	if usersStr := os.Getenv("ALLOWED_USERS"); usersStr != "" {
		for _, idStr := range strings.Split(usersStr, ",") {
			if id, err := fmt.Sscanf(strings.TrimSpace(idStr), "%d", new(int64)); err == nil && id == 1 {
				var userID int64
				fmt.Sscanf(strings.TrimSpace(idStr), "%d", &userID)
				allowedUsers = append(allowedUsers, userID)
			}
		}
		log.Printf("Allowed users: %v", allowedUsers)
	}

	// Load notify chat ID from env (so notifications work without waiting for a message)
	if chatIDStr := os.Getenv("NOTIFY_CHAT_ID"); chatIDStr != "" {
		fmt.Sscanf(strings.TrimSpace(chatIDStr), "%d", &notifyChatID)
		log.Printf("Notify chat ID loaded from env: %d", notifyChatID)
	}

	// Set bot commands automatically
	commands := []tgbotapi.BotCommand{
		// Estado
		{Command: "start", Description: "🐳 Menú principal"},
		{Command: "list", Description: "🐳 Todos los contenedores con estado"},
		{Command: "ps", Description: "🐳 Contenedores corriendo con CPU/RAM"},
		{Command: "stats", Description: "🐳 Dashboard del sistema"},
		// Gestión
		{Command: "create", Description: "🐳 Crear nuevo contenedor"},
		{Command: "restart", Description: "🐳 Reiniciar contenedor"},
		{Command: "stop", Description: "🐳 Detener contenedor"},
		{Command: "start_container", Description: "🐳 Iniciar contenedor detenido"},
		{Command: "pause", Description: "🐳 Pausar contenedor"},
		{Command: "unpause", Description: "🐳 Reanudar contenedor pausado"},
		// Logs y diagnóstico
		{Command: "logs", Description: "🐳 Ver logs de contenedor"},
		{Command: "logfile", Description: "🐳 Descargar logs como archivo .log"},
		{Command: "exec", Description: "🐳 Ejecutar comando en contenedor"},
		{Command: "diagnose", Description: "🐳 Diagnóstico automático del sistema"},
		// Compose y recursos
		{Command: "compose", Description: "🐳 Gestionar proyectos Docker Compose"},
		{Command: "inspect", Description: "🐳 Inspeccionar recursos Docker"},
		{Command: "images", Description: "🐳 Listar imágenes"},
		{Command: "volumes", Description: "🐳 Listar volúmenes"},
		{Command: "networks", Description: "🐳 Listar redes"},
		{Command: "prune", Description: "🐳 Limpiar recursos no usados"},
		// Actualizaciones
		{Command: "checkupdates", Description: "🐳 Buscar actualizaciones de imágenes"},
		{Command: "updateall", Description: "🐳 Actualizar todas las imágenes"},
		// Utilidades
		{Command: "search", Description: "🐳 Buscar contenedores/imágenes/volúmenes"},
		{Command: "env", Description: "🐳 Ver variables de entorno de un contenedor"},
		{Command: "port", Description: "🐳 Ver puertos expuestos de un contenedor"},
		{Command: "top", Description: "🐳 Procesos dentro de un contenedor"},
		{Command: "uptime", Description: "🐳 Tiempo corriendo de cada contenedor"},
		{Command: "rename", Description: "🐳 Renombrar un contenedor"},
		{Command: "copy", Description: "🐳 Duplicar un contenedor"},
		{Command: "backup", Description: "🐳 Exportar volumen como tar.gz"},
		{Command: "favorites", Description: "🐳 Ver contenedores favoritos"},
		{Command: "addfav", Description: "🐳 Agregar contenedor a favoritos"},
		{Command: "history", Description: "🐳 Historial de comandos"},
		{Command: "donate", Description: "🐳 Apoya el desarrollo del bot"},
	}
	
	cmdConfig := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(cmdConfig); err != nil {
		log.Printf("Error setting commands: %v", err)
	} else {
		log.Println("Bot commands configured successfully")
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
			log.Printf("Received message: %s from %d", update.Message.Text, update.Message.Chat.ID)
			
			// Check authentication
			if len(allowedUsers) > 0 {
				allowed := false
				for _, id := range allowedUsers {
					if id == update.Message.From.ID {
						allowed = true
						break
					}
				}
				if !allowed {
					sendMessageWithClose(update.Message.Chat.ID, "❌ No autorizado. Contacta al administrador.")
					log.Printf("Unauthorized access attempt from %d", update.Message.From.ID)
					continue
				}
			}
			
			// Log command
			if update.Message.Command() != "" {
				commandHistory[update.Message.From.ID] = append(commandHistory[update.Message.From.ID], update.Message.Command())
				if len(commandHistory[update.Message.From.ID]) > 50 {
					commandHistory[update.Message.From.ID] = commandHistory[update.Message.From.ID][1:]
				}
			}
			
			chatID := update.Message.Chat.ID
			userID := update.Message.From.ID
			notifyChatID = chatID
			
			// Delete the command message to keep chat clean
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, update.Message.MessageID)
			bot.Send(deleteMsg)
			
			// Check if user is in a conversation state
			if state, exists := userState[userID]; exists && (update.Message.Command() == "" || (strings.HasPrefix(state, "create_") && update.Message.Command() == "skip")) {
				text := update.Message.Text

				// Handle create container states
				if strings.HasPrefix(state, "create_") {
					go processCreateStep(chatID, userID, text)
					continue
				}

				// Handle exec manual command
				if strings.HasPrefix(state, "waiting_exec_cmd:") {
					container := strings.TrimPrefix(state, "waiting_exec_cmd:")
					delete(userState, userID)
					go func(c, cmd string) {
						cmdParts := strings.Fields(cmd)
						out, err := runCmd("docker", append([]string{"exec", c}, cmdParts...)...)
						var result string
						if err != nil {
							result = fmt.Sprintf("❌ Error: %s\n```\n%s\n```", err.Error(), out)
						} else {
							if len(out) > 3800 {
								out = out[:3800] + "\n...(truncado)"
							}
							result = fmt.Sprintf("⚙️ *%s* → `%s`\n```\n%s\n```", c, cmd, out)
						}
						sendMessageWithClose(chatID, result)
					}(container, text)
					continue
				}

				// Handle rename
				if strings.HasPrefix(state, "waiting_rename:") {
					old := strings.TrimPrefix(state, "waiting_rename:")
					delete(userState, userID)
					go func(oldName, newName string) {
						_, err := runCmd("docker", "rename", oldName, newName)
						if err != nil {
							sendMessageWithClose(chatID, "❌ Error al renombrar: "+err.Error())
						} else {
							sendMessageWithClose(chatID, fmt.Sprintf("✅ *%s* renombrado a *%s*", oldName, newName))
						}
					}(old, text)
					continue
				}

				// Handle copy
				if strings.HasPrefix(state, "waiting_copy:") {
					src := strings.TrimPrefix(state, "waiting_copy:")
					delete(userState, userID)
					go func(srcName, newName string) {
						// Get image of source container
						imgOut, err := runCmd("docker", "inspect", "--format", "{{.Config.Image}}", srcName)
						if err != nil {
							sendMessageWithClose(chatID, "❌ Error al inspeccionar: "+err.Error())
							return
						}
						image := strings.TrimSpace(imgOut)
						_, err = runCmd("docker", "run", "-d", "--name", newName, image)
						if err != nil {
							sendMessageWithClose(chatID, "❌ Error al copiar: "+err.Error())
						} else {
							sendMessageWithClose(chatID, fmt.Sprintf("✅ Contenedor *%s* creado como copia de *%s*\n📦 Imagen: `%s`", newName, srcName, image))
						}
					}(src, text)
					continue
				}

				delete(userState, userID) // Clear state after processing

				switch state {
				case "waiting_search":
					go handleSearch(chatID, text)
					continue
				}
			}
			
			switch update.Message.Command() {
			case "start":
				go handleStart(chatID)
			case "ps":
				go handlePS(chatID)
			case "running":
				go handleRunning(chatID)
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
			case "notify":
				notifyChatID = chatID
				sendMessageWithClose(chatID, "✅ Notificaciones activadas")
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
					sendMessageWithClose(chatID, "🔍 ¿Qué deseas buscar?\nEscribe el término a buscar:")
				} else {
					go handleSearch(chatID, update.Message.CommandArguments())
				}
			case "pause":
				go handlePauseMenu(chatID)
			case "unpause":
				go handleUnpauseMenu(chatID)
			case "favorites":
				go handleFavorites(chatID, update.Message.From.ID)
			case "addfav":
				go handleAddFavoriteMenu(chatID, userID)
			case "env":
				go handleEnvMenu(chatID)
			case "history":
				go handleHistory(chatID, update.Message.From.ID)
			case "diagnose":
				go handleDiagnose(chatID)
			case "list":
				go handleList(chatID)
			case "donate":
				msg := tgbotapi.NewMessage(chatID, "☕ *Apoya Botainer*\n\nSi el bot te resulta útil, considera apoyar el desarrollo:\n\n💛 [Buy Me a Coffee](https://buymeacoffee.com/yoniergomez)\n💖 [GitHub Sponsors](https://github.com/sponsors/YonierGomez)\n\nCada contribución ayuda a mantener el proyecto activo. ¡Gracias! 🙏")
				msg.ParseMode = "Markdown"
				msg.DisableWebPagePreview = true
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonURL("☕ Buy Me a Coffee", "https://buymeacoffee.com/yoniergomez"),
					),
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonURL("💖 GitHub Sponsors", "https://github.com/sponsors/YonierGomez"),
					),
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
					),
				)
				bot.Send(msg)
			case "checkupdates":
				go func() {
					sendMessageWithClose(chatID, "🔍 Buscando actualizaciones de imágenes...")
					runImageUpdateCheckWithFeedback(chatID)
				}()
			case "port":
				go handleGrid(chatID, "🔌 *Ver puertos*\nSelecciona un contenedor:", "ports", false)
			case "top":
				go handleGrid(chatID, "🔝 *Procesos*\nSelecciona un contenedor:", "top", false)
			case "uptime":
				go handleUptime(chatID)
			case "rename":
				go handleGrid(chatID, "✏️ *Renombrar contenedor*\nSelecciona un contenedor:", "rename_prompt", true)
			case "copy":
				go handleGrid(chatID, "📋 *Copiar contenedor*\nSelecciona un contenedor:", "copy_prompt", false)
			case "backup":
				go handleBackupMenu(chatID)
			}
		} else if update.CallbackQuery != nil {
			log.Printf("Received callback: %s from %d", update.CallbackQuery.Data, update.CallbackQuery.Message.Chat.ID)
			go handleCallback(update.CallbackQuery)
		}
	}
}

func handleStartContainer(chatID int64) {
	out, err := runCmd("docker", "ps", "-a", "--filter", "status=exited", "--format", "{{.Names}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores detenidos")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		icon1 := getIcon(lines[i])
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+lines[i], "start:"+lines[i]),
		}
		if i+1 < len(lines) {
			icon2 := getIcon(lines[i+1])
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+lines[i+1], "start:"+lines[i+1]))
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
	// Get system info
	dfOut, _ := runCmd("df", "-h", "/")
	memOut, _ := runCmd("free", "-h")
	
	// Count resources
	containersOut, _ := runCmd("docker", "ps", "-a", "-q")
	runningOut, _ := runCmd("docker", "ps", "-q")
	imagesOut, _ := runCmd("docker", "images", "-q")
	volumesOut, _ := runCmd("docker", "volume", "ls", "-q")
	networksOut, _ := runCmd("docker", "network", "ls", "-q")

	containerCount := len(strings.Split(strings.TrimSpace(containersOut), "\n"))
	runningCount := len(strings.Split(strings.TrimSpace(runningOut), "\n"))
	imageCount := len(strings.Split(strings.TrimSpace(imagesOut), "\n"))
	volumeCount := len(strings.Split(strings.TrimSpace(volumesOut), "\n"))
	networkCount := len(strings.Split(strings.TrimSpace(networksOut), "\n"))
	
	// Parse disk usage
	diskLines := strings.Split(dfOut, "\n")
	diskInfo := "N/A"
	if len(diskLines) > 1 {
		fields := strings.Fields(diskLines[1])
		if len(fields) >= 5 {
			diskInfo = fmt.Sprintf("%s / %s (%s usado)", fields[2], fields[1], fields[4])
		}
	}
	
	// Parse memory
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
  • Redes: %d`, diskInfo, memInfo, containerCount, runningCount, imageCount, volumeCount, networkCount)
	
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

// recreateContainer pulls the latest image and recreates the container.
// For compose containers it uses docker compose up -d (keeps compose management).
// For standalone containers it uses docker pull + docker restart (no docker run).
func recreateContainer(name string) error {
	// Inspect to get image and compose labels
	inspectOut, err := runCmd("docker", "inspect", name)
	if err != nil {
		return fmt.Errorf("inspect failed: %w", err)
	}
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(inspectOut), &data); err != nil || len(data) == 0 {
		return fmt.Errorf("parse inspect failed")
	}
	container := data[0]
	config, _ := container["Config"].(map[string]interface{})
	image, _ := config["Image"].(string)
	if image == "" {
		return fmt.Errorf("could not get image name")
	}

	// Pull new image first (lightweight, only this image)
	if _, err := runCmd("docker", "pull", image); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// Check if it's a compose container
	var project, workDir, service string
	if labels, ok := config["Labels"].(map[string]interface{}); ok {
		project, _ = labels["com.docker.compose.project"].(string)
		workDir, _ = labels["com.docker.compose.project.working_dir"].(string)
		service, _ = labels["com.docker.compose.service"].(string)
	}

	if project != "" && workDir != "" && service != "" {
		// Remap host path to container workspace
		workspace := os.Getenv("WORKSPACE")
		if workspace == "" {
			workspace = "/workspace"
		}
		hostHome := os.Getenv("HOST_HOME")
		if hostHome == "" {
			hostHome = "/home/ubuntu"
		}
		workDir = strings.Replace(workDir, hostHome, workspace, 1)
		// docker compose up -d <service> — only recreates this service, no pull
		_, err := runCmd("docker", "compose", "--project-directory", workDir, "-p", project, "up", "-d", service)
		return err
	}

	// Standalone: pull already done, just restart to pick up new image
	// Note: docker restart does NOT use new image — but without compose we can't
	// safely recreate. Best we can do is inform the user.
	_, err = runCmd("docker", "restart", name)
	return err
}

func getComposeWorkDir(project string) string {
	psOut, _ := runCmd("docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}")
	for _, cname := range strings.Split(strings.TrimSpace(psOut), "\n") {
		if cname == "" {
			continue
		}
		inspOut, _ := runCmd("docker", "inspect", cname)
		var inspData []map[string]interface{}
		if json.Unmarshal([]byte(inspOut), &inspData) == nil && len(inspData) > 0 {
			if labels, ok := inspData[0]["Config"].(map[string]interface{})["Labels"].(map[string]interface{}); ok {
				if wd, ok := labels["com.docker.compose.project.working_dir"].(string); ok && wd != "" {
					// Remap host path to container workspace mount
					// e.g. /home/ubuntu/chips_all -> /workspace/chips_all
					workspace := os.Getenv("WORKSPACE")
					if workspace == "" {
						workspace = "/workspace"
					}
					hostHome := os.Getenv("HOST_HOME")
					if hostHome == "" {
						hostHome = "/home/ubuntu"
					}
					return strings.Replace(wd, hostHome, workspace, 1)
				}
			}
		}
	}
	return ""
}

func handleCompose(chatID int64) {
	// Find all compose projects
	out, _ := runCmd("docker", "ps", "-a", "--format", "{{.Label \"com.docker.compose.project\"}}")
	
	projectsMap := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		project := strings.TrimSpace(line)
		if project != "" {
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
	
	query = strings.ToLower(query)
	results := []string{}
	
	// Search containers
	out, _ := runCmd("docker", "ps", "-a", "--format", "{{.Names}}|{{.Image}}|{{.Status}}")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), query) {
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				results = append(results, fmt.Sprintf("📦 %s (`%s`)", parts[0], parts[1]))
			}
		}
	}
	
	// Search images
	imgOut, _ := runCmd("docker", "images", "--format", "{{.Repository}}:{{.Tag}}")
	for _, line := range strings.Split(imgOut, "\n") {
		if strings.Contains(strings.ToLower(line), query) {
			results = append(results, fmt.Sprintf("🖼️ %s", line))
		}
	}
	
	// Search volumes
	volOut, _ := runCmd("docker", "volume", "ls", "--format", "{{.Name}}")
	for _, line := range strings.Split(volOut, "\n") {
		if strings.Contains(strings.ToLower(line), query) {
			results = append(results, fmt.Sprintf("💾 %s", line))
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
	out, _ := runCmd("docker", "ps", "--filter", "status=paused", "--format", "{{.Names}}")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores pausados")
		return
	}
	
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		icon1 := getIcon(lines[i])
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+lines[i], "unpause:"+lines[i]),
		}
		if i+1 < len(lines) {
			icon2 := getIcon(lines[i+1])
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+lines[i+1], "unpause:"+lines[i+1]))
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
		sendMessageWithClose(chatID, "No tienes favoritos.\nUsa /addfav <contenedor> para agregar.")
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

func handleAddFavorite(chatID int64, userID int64, container string) {
	if container == "" {
		return
	}
	
	// Check if container exists
	out, _ := runCmd("docker", "ps", "-a", "--filter", "name="+container, "--format", "{{.Names}}")
	if strings.TrimSpace(out) == "" {
		sendMessageWithClose(chatID, fmt.Sprintf("❌ Contenedor *%s* no encontrado", container))
		return
	}
	
	// Add to favorites
	for _, fav := range favorites[userID] {
		if fav == container {
			sendMessageWithClose(chatID, fmt.Sprintf("⭐ *%s* ya está en favoritos", container))
			return
		}
	}
	
	favorites[userID] = append(favorites[userID], container)
	sendMessageWithClose(chatID, fmt.Sprintf("✅ *%s* agregado a favoritos", container))
}

func handleAddFavoriteMenu(chatID int64, userID int64) {
	out, _ := runCmd("docker", "ps", "-a", "--format", "{{.Names}}")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores")
		return
	}
	
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		icon1 := getIcon(lines[i])
		// Check if already in favorites
		isFav1 := false
		for _, fav := range favorites[userID] {
			if fav == lines[i] {
				isFav1 = true
				break
			}
		}
		
		label1 := icon1 + " " + lines[i]
		if isFav1 {
			label1 = "✅ " + label1
		}
		
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label1, "togglefav:"+lines[i]),
		}
		
		if i+1 < len(lines) {
			icon2 := getIcon(lines[i+1])
			isFav2 := false
			for _, fav := range favorites[userID] {
				if fav == lines[i+1] {
					isFav2 = true
					break
				}
			}
			
			label2 := icon2 + " " + lines[i+1]
			if isFav2 {
				label2 = "✅ " + label2
			}
			
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label2, "togglefav:"+lines[i+1]))
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
	out, _ := runCmd("docker", "ps", "--format", "{{.Names}}")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores corriendo")
		return
	}
	
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		icon1 := getIcon(lines[i])
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(icon1+" "+lines[i], "env:"+lines[i]),
		}
		if i+1 < len(lines) {
			icon2 := getIcon(lines[i+1])
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(icon2+" "+lines[i+1], "env:"+lines[i+1]))
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
	
	// Show last 20 commands
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
	
	sendMessageWithClose(chatID, "📦 *Crear contenedor con Docker Run*\n\n1️⃣ Escribe el nombre de la imagen:\nEjemplo: `nginx:latest`, `postgres:15`, `redis:alpine`")
}

func handleCreateCompose(chatID int64, userID int64) {
	createData[userID] = make(map[string]string)
	createData[userID]["type"] = "compose"
	userState[userID] = "create_service_name"
	
	sendMessageWithClose(chatID, "🐙 *Crear contenedor con Docker Compose*\n\n1️⃣ Escribe el nombre del servicio:\nEjemplo: `web`, `database`, `cache`")
}

func processCreateStep(chatID int64, userID int64, text string) {
	state := userState[userID]
	data := createData[userID]
	
	switch state {
	case "create_image":
		data["image"] = text
		userState[userID] = "create_name"
		sendMessageWithClose(chatID, "2️⃣ Escribe el nombre del contenedor:\nEjemplo: `mi-nginx`, `mi-postgres`\n\n_Presiona /skip para generar automáticamente_")
		
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
		sendMessageWithClose(chatID, "4️⃣ Escribe los volúmenes (opcional):\nEjemplo: `/data:/app/data`, `vol1:/var/lib`\n\n_Presiona /skip para omitir_")
		
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
		sendMessageWithClose(chatID, "2️⃣ Escribe el nombre de la imagen:\nEjemplo: `nginx:latest`, `postgres:15`")
		
	case "create_compose_image":
		data["image"] = text
		userState[userID] = "create_compose_ports"
		sendMessageWithClose(chatID, "3️⃣ Escribe los puertos (opcional):\nEjemplo: `80:80`, `8080:80,3306:3306`\n\n_Presiona /skip para omitir_")
		
	case "create_compose_ports":
		if text != "/skip" {
			data["ports"] = text
		}
		userState[userID] = "create_compose_volumes"
		sendMessageWithClose(chatID, "4️⃣ Escribe los volúmenes (opcional):\nEjemplo: `/data:/app/data`, `vol1:/var/lib`\n\n_Presiona /skip para omitir_")
		
	case "create_compose_volumes":
		if text != "/skip" {
			data["volumes"] = text
		}
		userState[userID] = "create_compose_env"
		sendMessageWithClose(chatID, "5️⃣ Escribe las variables de entorno (opcional):\nEjemplo: `DB_USER=admin,DB_PASS=secret`\n\n_Presiona /skip para omitir_")
		
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
	issues := []string{}
	
	// Check stopped containers
	stoppedOut, _ := runCmd("docker", "ps", "-a", "--filter", "status=exited", "--format", "{{.Names}}")
	stopped := strings.Split(strings.TrimSpace(stoppedOut), "\n")
	if len(stopped) > 0 && stopped[0] != "" {
		issues = append(issues, fmt.Sprintf("⚠️ %d contenedores detenidos", len(stopped)))
	}
	
	// Check unhealthy containers
	unhealthyOut, _ := runCmd("docker", "ps", "--filter", "health=unhealthy", "--format", "{{.Names}}")
	unhealthy := strings.Split(strings.TrimSpace(unhealthyOut), "\n")
	if len(unhealthy) > 0 && unhealthy[0] != "" {
		issues = append(issues, fmt.Sprintf("❤️ %d contenedores no saludables: %s", len(unhealthy), strings.Join(unhealthy, ", ")))
	}
	
	// Check high CPU usage
	statsOut, _ := runCmd("docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}")
	for _, line := range strings.Split(statsOut, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			cpuStr := strings.TrimSuffix(parts[1], "%")
			var cpu float64
			if _, err := fmt.Sscanf(cpuStr, "%f", &cpu); err == nil && cpu > 80 {
				issues = append(issues, fmt.Sprintf("🔥 %s usando %s CPU", parts[0], parts[1]))
			}
		}
	}
	
	// Check dangling images
	danglingOut, _ := runCmd("docker", "images", "-f", "dangling=true", "-q")
	dangling := strings.Split(strings.TrimSpace(danglingOut), "\n")
	if len(dangling) > 0 && dangling[0] != "" {
		issues = append(issues, fmt.Sprintf("🗑️ %d imágenes sin usar (ejecuta /prune)", len(dangling)))
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
	out, err := runCmd("docker", "ps", "--format", "{{.Names}}|{{.Status}}")
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error: "+err.Error())
		return
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		sendMessageWithClose(chatID, "No hay contenedores corriendo")
		return
	}
	text := "⏱️ *Uptime de contenedores*\n\n"
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) < 2 {
			continue
		}
		name, status := parts[0], parts[1]
		icon := getIcon(name)
		// docker ps status already contains uptime e.g. "Up 3 days"
		text += fmt.Sprintf("%s *%s*\n   └ `%s`\n", icon, name, status)
	}
	sendMessageWithClose(chatID, text)
}

func handleBackupMenu(chatID int64) {
	out, err := runCmd("docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil || strings.TrimSpace(out) == "" {
		sendMessageWithClose(chatID, "No hay volúmenes disponibles")
		return
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(lines); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("💾 "+lines[i], "backup:"+lines[i]),
		}
		if i+1 < len(lines) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("💾 "+lines[i+1], "backup:"+lines[i+1]))
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
