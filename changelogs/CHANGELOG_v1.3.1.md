# Changelog v1.3.1

## 🐛 Bug Fixes Release

This release fixes critical issues with CPU statistics and improves user experience.

---

## 🔧 Fixes

### CPU Statistics
**Problem:** CPU usage was showing 0.0% or N/A for all containers in `/ps` and `/alerts` commands.

**Root Cause:** The `PercpuUsage` array was empty when using `stream=false`, causing the CPU calculation to fail.

**Solution:** Use the `OnlineCPUs` field from Docker SDK which correctly reports the number of CPUs (8 in most systems). Falls back to `PercpuUsage` length if `OnlineCPUs` is 0.

**Impact:** CPU statistics now display correctly for all containers using pure Docker SDK.

### Memory Display
**Changed:** Memory now displays in GB instead of MiB for better readability.

**Before:**
```
RAM: 112MiB / 15957MiB
```

**After:**
```
RAM: 0.11GB / 15.58GB
```

### Low CPU Containers
**Fixed:** Containers with very low CPU usage (0.00%) were showing "N/A" instead of "0.0%".

**Now:** Correctly displays "0.0%" for idle containers, matching `docker stats` output.

### Telegram Notifications
**Fixed:** Channel notifications were truncated and showing raw markdown.

**Improvements:**
- Clean, short format (under 1000 characters)
- Shows last 5 commits as changelog
- Proper code blocks for update commands
- Link to full release notes

**New Format:**
```
🚀 Botainer X.X.X disponible

✨ Novedades:
• Last 5 commits here

🐳 Actualizar:
```bash
docker pull yoniergomez/botainer:latest
docker compose up -d --build botainer
```

📖 Ver release completo
```

---

## 📊 Technical Details

### CPU Calculation Fix
```go
// Use OnlineCPUs field (reliable)
numCPU := v.CPUStats.OnlineCPUs
if numCPU == 0 {
    numCPU = uint32(len(v.CPUStats.CPUUsage.PercpuUsage))
}

if systemDelta > 0 && numCPU > 0 {
    cpuPercent = (cpuDelta / systemDelta) * float64(numCPU) * 100.0
}
```

### Memory Conversion
```go
// Convert to GB
memUsage := float64(v.MemoryStats.Usage) / 1024 / 1024 / 1024
memLimit := float64(v.MemoryStats.Limit) / 1024 / 1024 / 1024
mem = fmt.Sprintf("%.2fGB / %.2fGB", memUsage, memLimit)
```

---

## 🚀 Upgrade Instructions

```bash
cd botainer
git pull
docker compose up -d --build
```

Or pull the latest image:

```bash
docker pull yoniergomez/botainer:latest
docker compose up -d
```

---

## 📝 Commits in this Release

- feat: display memory in GB instead of MiB
- fix: show 0.0% instead of N/A for low CPU usage containers
- fix: use OnlineCPUs field from Docker SDK for accurate CPU calculation
- fix: simplify Telegram channel notifications
- feat: add changelog summary to Telegram notifications

---

**Version**: 1.3.1  
**Date**: 2026-05-05  
**Type**: Bug Fix Release  
**Breaking Changes**: None  
**Migration Required**: No
