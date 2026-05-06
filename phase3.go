package main

// Phase 3: Vulnerability scanning, Audit logs, Webhooks, Auto-update policies

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ═══════════════════════════════════════════════════════════════════════════
// AUDIT LOG
// ═══════════════════════════════════════════════════════════════════════════

const auditLogFile = "/data/audit.log"

type AuditEntry struct {
	Time    string `json:"time"`
	UserID  int64  `json:"user_id"`
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
}

var auditMu sync.Mutex

func auditLog(userID int64, command, target string) {
	entry := AuditEntry{
		Time:    time.Now().UTC().Format(time.RFC3339),
		UserID:  userID,
		Command: command,
		Target:  target,
	}
	line, _ := json.Marshal(entry)

	auditMu.Lock()
	defer auditMu.Unlock()

	os.MkdirAll("/data", 0755)
	f, err := os.OpenFile(auditLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[audit] error opening log: %v", err)
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

func handleAuditLog(chatID int64) {
	f, err := os.Open(auditLogFile)
	if err != nil {
		sendMessageWithClose(chatID, "📋 No hay entradas de auditoría aún.")
		return
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) == 0 {
		sendMessageWithClose(chatID, "📋 No hay entradas de auditoría aún.")
		return
	}

	// Show last 20 entries
	start := len(lines) - 20
	if start < 0 {
		start = 0
	}
	recent := lines[start:]

	var sb strings.Builder
	sb.WriteString("📋 *Audit Log* (últimas entradas)\n```\n")
	for _, l := range recent {
		var e AuditEntry
		if json.Unmarshal([]byte(l), &e) == nil {
			sb.WriteString(fmt.Sprintf("[%s] uid=%d cmd=%s", e.Time[:16], e.UserID, e.Command))
			if e.Target != "" {
				sb.WriteString(" target=" + e.Target)
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("```")

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📥 Exportar", "audit_export"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func handleAuditExport(chatID int64) {
	if _, err := os.Stat(auditLogFile); os.IsNotExist(err) {
		sendMessageWithClose(chatID, "📋 No hay entradas de auditoría para exportar.")
		return
	}
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(auditLogFile))
	doc.Caption = "📋 Audit log completo"
	bot.Send(doc)
}

// ═══════════════════════════════════════════════════════════════════════════
// VULNERABILITY SCANNING (Trivy)
// ═══════════════════════════════════════════════════════════════════════════

type TrivyResult struct {
	Results []struct {
		Target          string `json:"Target"`
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			Severity         string `json:"Severity"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			Title            string `json:"Title"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func trivyAvailable() bool {
	_, err := exec.LookPath("trivy")
	return err == nil
}

func scanImageWithTrivy(imageRef string) (string, error) {
	out, err := exec.Command("trivy", "image", "--format", "json", "--quiet", "--timeout", "120s", imageRef).Output()
	if err != nil {
		// trivy exits non-zero when vulns found; check if we got output
		if len(out) == 0 {
			return "", fmt.Errorf("trivy error: %v", err)
		}
	}

	var result TrivyResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parse error: %v", err)
	}

	counts := map[string]int{"CRITICAL": 0, "HIGH": 0, "MEDIUM": 0, "LOW": 0}
	var criticals []string

	for _, r := range result.Results {
		for _, v := range r.Vulnerabilities {
			counts[v.Severity]++
			if v.Severity == "CRITICAL" && len(criticals) < 5 {
				criticals = append(criticals, fmt.Sprintf("• `%s` (%s) — %s", v.VulnerabilityID, v.PkgName, v.Title))
			}
		}
	}

	total := counts["CRITICAL"] + counts["HIGH"] + counts["MEDIUM"] + counts["LOW"]
	if total == 0 {
		return fmt.Sprintf("✅ *%s*\nSin vulnerabilidades conocidas.", imageRef), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Scan: %s*\n", imageRef))
	sb.WriteString(fmt.Sprintf("🔴 CRITICAL: %d | 🟠 HIGH: %d | 🟡 MEDIUM: %d | 🟢 LOW: %d\n", counts["CRITICAL"], counts["HIGH"], counts["MEDIUM"], counts["LOW"]))
	if len(criticals) > 0 {
		sb.WriteString("\n*CVEs críticos:*\n")
		sb.WriteString(strings.Join(criticals, "\n"))
	}
	return sb.String(), nil
}

func handleScanMenu(chatID int64) {
	if !trivyAvailable() {
		sendMessageWithClose(chatID, "❌ Trivy no está instalado.\nInstálalo con: `apt install trivy` o visita https://trivy.dev")
		return
	}

	images, err := cli.ImageList(context.Background(), image.ListOptions{})
	if err != nil || len(images) == 0 {
		sendMessageWithClose(chatID, "No hay imágenes locales para escanear.")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for _, img := range images {
		if len(img.RepoTags) == 0 {
			continue
		}
		tag := img.RepoTags[0]
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 "+tag, "scan:"+tag),
		))
		if len(keyboard) >= 10 {
			break
		}
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "🔍 *Escaneo de vulnerabilidades*\nSelecciona una imagen:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleScanImage(chatID int64, imageRef string) {
	loadingID := sendLoading(chatID, fmt.Sprintf("Escaneando `%s` con Trivy...", imageRef))
	result, err := scanImageWithTrivy(imageRef)
	deleteMsg(chatID, loadingID)
	if err != nil {
		sendMessageWithClose(chatID, "❌ Error al escanear: "+err.Error())
		return
	}
	// Fire webhook
	go fireWebhook("scan", map[string]string{"image": imageRef, "result": result})
	sendMessageWithClose(chatID, result)
}

// ═══════════════════════════════════════════════════════════════════════════
// WEBHOOKS
// ═══════════════════════════════════════════════════════════════════════════

var (
	webhookURLs   []string
	webhookMu     sync.RWMutex
	webhookConfig = "/data/webhooks.json"
)

func loadWebhooks() {
	data, err := os.ReadFile(webhookConfig)
	if err != nil {
		return
	}
	webhookMu.Lock()
	defer webhookMu.Unlock()
	json.Unmarshal(data, &webhookURLs)
}

func saveWebhooks() {
	webhookMu.RLock()
	defer webhookMu.RUnlock()
	data, _ := json.Marshal(webhookURLs)
	os.WriteFile(webhookConfig, data, 0644)
}

func fireWebhook(event string, payload map[string]string) {
	webhookMu.RLock()
	urls := make([]string, len(webhookURLs))
	copy(urls, webhookURLs)
	webhookMu.RUnlock()

	if len(urls) == 0 {
		return
	}

	body := map[string]interface{}{
		"event":   event,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"payload": payload,
	}
	data, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	for _, u := range urls {
		resp, err := client.Post(u, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("[webhook] error sending to %s: %v", u, err)
			continue
		}
		resp.Body.Close()
		log.Printf("[webhook] sent event=%s to %s status=%d", event, u, resp.StatusCode)
	}
}

func handleWebhookMenu(chatID int64) {
	webhookMu.RLock()
	count := len(webhookURLs)
	webhookMu.RUnlock()

	text := fmt.Sprintf("🔔 *Webhooks* (%d configurados)\n\nEnvía notificaciones a URLs externas en eventos Docker.", count)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Agregar", "webhook_add"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Listar", "webhook_list"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ Limpiar todos", "webhook_clear"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func handleWebhookList(chatID int64) {
	webhookMu.RLock()
	urls := make([]string, len(webhookURLs))
	copy(urls, webhookURLs)
	webhookMu.RUnlock()

	if len(urls) == 0 {
		sendMessageWithClose(chatID, "No hay webhooks configurados.")
		return
	}

	var sb strings.Builder
	sb.WriteString("🔔 *Webhooks configurados:*\n")
	for i, u := range urls {
		sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, u))
	}
	sendMessageWithClose(chatID, sb.String())
}

func addWebhookURL(chatID int64, url string) {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		sendMessageWithClose(chatID, "❌ URL inválida. Debe comenzar con http:// o https://")
		return
	}
	webhookMu.Lock()
	webhookURLs = append(webhookURLs, url)
	webhookMu.Unlock()
	saveWebhooks()
	sendMessageWithClose(chatID, fmt.Sprintf("✅ Webhook agregado:\n`%s`", url))
}

// ═══════════════════════════════════════════════════════════════════════════
// AUTO-UPDATE POLICIES
// ═══════════════════════════════════════════════════════════════════════════

type UpdatePolicy struct {
	Schedule    string  `json:"schedule"`    // "daily", "weekly", "off"
	MaxCPU      float64 `json:"max_cpu"`     // skip update if CPU% above this (0 = no limit)
	MaxMemPct   float64 `json:"max_mem_pct"` // skip update if mem% above this (0 = no limit)
}

var (
	updatePolicies   = make(map[string]UpdatePolicy) // container -> policy
	updatePoliciesMu sync.Mutex
	policiesFile     = "/data/policies.json"
)

func loadPolicies() {
	data, err := os.ReadFile(policiesFile)
	if err != nil {
		return
	}
	updatePoliciesMu.Lock()
	defer updatePoliciesMu.Unlock()
	json.Unmarshal(data, &updatePolicies)
}

func savePolicies() {
	updatePoliciesMu.Lock()
	defer updatePoliciesMu.Unlock()
	data, _ := json.MarshalIndent(updatePolicies, "", "  ")
	os.WriteFile(policiesFile, data, 0644)
}

func handlePolicyMenu(chatID int64) {
	updatePoliciesMu.Lock()
	count := len(updatePolicies)
	updatePoliciesMu.Unlock()

	text := fmt.Sprintf("⚙️ *Políticas de actualización* (%d configuradas)\n\nDefine cuándo y bajo qué condiciones se actualizan los contenedores.", count)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Ver políticas", "policy_list"),
			tgbotapi.NewInlineKeyboardButtonData("➕ Configurar", "policy_set_menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("▶️ Ejecutar ahora", "policy_run"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}

func handlePolicyList(chatID int64) {
	updatePoliciesMu.Lock()
	policies := make(map[string]UpdatePolicy)
	for k, v := range updatePolicies {
		policies[k] = v
	}
	updatePoliciesMu.Unlock()

	if len(policies) == 0 {
		sendMessageWithClose(chatID, "No hay políticas configuradas.")
		return
	}

	var sb strings.Builder
	sb.WriteString("⚙️ *Políticas de actualización:*\n\n")
	for name, p := range policies {
		sb.WriteString(fmt.Sprintf("📦 *%s*\n   Schedule: `%s`", name, p.Schedule))
		if p.MaxCPU > 0 {
			sb.WriteString(fmt.Sprintf(" | MaxCPU: `%.0f%%`", p.MaxCPU))
		}
		if p.MaxMemPct > 0 {
			sb.WriteString(fmt.Sprintf(" | MaxMem: `%.0f%%`", p.MaxMemPct))
		}
		sb.WriteString("\n")
	}
	sendMessageWithClose(chatID, sb.String())
}

func setPolicyForContainer(container, schedule string, maxCPU, maxMem float64) {
	updatePoliciesMu.Lock()
	updatePolicies[container] = UpdatePolicy{Schedule: schedule, MaxCPU: maxCPU, MaxMemPct: maxMem}
	updatePoliciesMu.Unlock()
	savePolicies()
}

// getSystemCPUPercent returns approximate system CPU usage
func getSystemCPUPercent() float64 {
	out, err := exec.Command("sh", "-c", "top -bn1 | grep 'Cpu(s)' | awk '{print $2+$4}'").Output()
	if err != nil {
		return 0
	}
	var pct float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &pct)
	return pct
}

// getSystemMemPercent returns approximate system memory usage %
func getSystemMemPercent() float64 {
	out, err := exec.Command("sh", "-c", "free | awk '/Mem:/{printf \"%.1f\", $3/$2*100}'").Output()
	if err != nil {
		return 0
	}
	var pct float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &pct)
	return pct
}

// runPolicyUpdates checks policies and updates eligible containers
func runPolicyUpdates(chatID int64) {
	updatePoliciesMu.Lock()
	policies := make(map[string]UpdatePolicy)
	for k, v := range updatePolicies {
		policies[k] = v
	}
	updatePoliciesMu.Unlock()

	if len(policies) == 0 {
		if chatID != 0 {
			sendMessageWithClose(chatID, "No hay políticas configuradas.")
		}
		return
	}

	cpuPct := getSystemCPUPercent()
	memPct := getSystemMemPercent()

	var updated, skipped []string

	for name, policy := range policies {
		if policy.Schedule == "off" {
			skipped = append(skipped, name+" (off)")
			continue
		}
		if policy.MaxCPU > 0 && cpuPct > policy.MaxCPU {
			skipped = append(skipped, fmt.Sprintf("%s (CPU %.0f%% > %.0f%%)", name, cpuPct, policy.MaxCPU))
			continue
		}
		if policy.MaxMemPct > 0 && memPct > policy.MaxMemPct {
			skipped = append(skipped, fmt.Sprintf("%s (Mem %.0f%% > %.0f%%)", name, memPct, policy.MaxMemPct))
			continue
		}

		if err := recreateWithNewImage(name); err != nil {
			skipped = append(skipped, name+" (error: "+err.Error()+")")
		} else {
			updated = append(updated, name)
			auditLog(0, "policy_update", name)
			go fireWebhook("policy_update", map[string]string{"container": name, "schedule": policy.Schedule})
		}
	}

	if chatID != 0 {
		var sb strings.Builder
		sb.WriteString("⚙️ *Ejecución de políticas*\n")
		if len(updated) > 0 {
			sb.WriteString(fmt.Sprintf("✅ Actualizados: %s\n", strings.Join(updated, ", ")))
		}
		if len(skipped) > 0 {
			sb.WriteString(fmt.Sprintf("⏭️ Omitidos: %s\n", strings.Join(skipped, ", ")))
		}
		sendMessageWithClose(chatID, sb.String())
	}
}

// scheduledPolicyRunner runs policy updates on schedule
func scheduledPolicyRunner() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		updatePoliciesMu.Lock()
		policies := make(map[string]UpdatePolicy)
		for k, v := range updatePolicies {
			policies[k] = v
		}
		updatePoliciesMu.Unlock()

		for name, policy := range policies {
			shouldRun := false
			switch policy.Schedule {
			case "daily":
				shouldRun = now.Hour() == 3 // 3 AM
			case "weekly":
				shouldRun = now.Weekday() == time.Sunday && now.Hour() == 3
			}
			if !shouldRun {
				continue
			}

			cpuPct := getSystemCPUPercent()
			memPct := getSystemMemPercent()
			if policy.MaxCPU > 0 && cpuPct > policy.MaxCPU {
				log.Printf("[policy] skipping %s: CPU %.0f%% > %.0f%%", name, cpuPct, policy.MaxCPU)
				continue
			}
			if policy.MaxMemPct > 0 && memPct > policy.MaxMemPct {
				log.Printf("[policy] skipping %s: Mem %.0f%% > %.0f%%", name, memPct, policy.MaxMemPct)
				continue
			}

			log.Printf("[policy] updating %s (schedule=%s)", name, policy.Schedule)
			if err := recreateWithNewImage(name); err != nil {
				log.Printf("[policy] error updating %s: %v", name, err)
			} else {
				auditLog(0, "scheduled_update", name)
				go fireWebhook("scheduled_update", map[string]string{"container": name, "schedule": policy.Schedule})
				if notifyChatID != 0 {
					bot.Send(tgbotapi.NewMessage(notifyChatID, fmt.Sprintf("⚙️ *Política automática*: `%s` actualizado (schedule: %s)", name, policy.Schedule)))
				}
			}
		}
	}
}

// handlePolicySetMenu shows container list to set policy on
func handlePolicySetMenu(chatID int64) {
	containers, err := cli.ContainerList(context.Background(), container.ListOptions{})
	if err != nil || len(containers) == 0 {
		sendMessageWithClose(chatID, "No hay contenedores disponibles.")
		return
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📦 "+name, "policy_container:"+name),
		))
	}
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
	))

	msg := tgbotapi.NewMessage(chatID, "⚙️ *Selecciona un contenedor para configurar política:*")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handlePolicyContainerSchedule(chatID int64, containerName string) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚙️ *Política para %s*\nSelecciona el schedule:", containerName))
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Diario (3 AM)", "policy_apply:"+containerName+":daily"),
			tgbotapi.NewInlineKeyboardButtonData("📆 Semanal (Dom 3 AM)", "policy_apply:"+containerName+":weekly"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚫 Desactivar", "policy_apply:"+containerName+":off"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cerrar", "close"),
		),
	)
	bot.Send(msg)
}
