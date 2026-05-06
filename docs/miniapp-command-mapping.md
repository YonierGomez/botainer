# Mini App Command Mapping

Este documento detalla cómo cada comando del bot actual se traducirá a la interfaz visual de la Mini App.

## Estrategia General

- **Bot commands**: Se mantienen para acceso rápido y usuarios que prefieren CLI
- **Mini App**: Interfaz visual rica para operaciones complejas
- **Coexistencia**: Ambos métodos funcionan simultáneamente

---

## 📊 Menu & Status Commands

### `/start` → **Home Dashboard**
**Bot**: Menú con botones
**Mini App**: 
- Dashboard principal con cards
- Resumen: X contenedores running, Y stopped, Z paused
- Gráfico de torta: distribución de estados
- Accesos rápidos a secciones principales

### `/list` → **Container List View**
**Bot**: Lista con botones (2 columnas)
**Mini App**:
- Tabla interactiva con columnas: Name, Status, Image, CPU, RAM, Uptime
- Filtros: All / Running / Stopped / Paused
- Búsqueda en tiempo real
- Click en fila → Container Detail View
- Checkbox para bulk operations

### `/ps` → **Resource Monitor**
**Bot**: Texto con CPU/RAM por contenedor
**Mini App**:
- Cards con gauge charts para cada contenedor
- Ordenar por: CPU, RAM, Name
- Gráficos de barras comparativos
- Refresh automático cada 5s

### `/running` → **Quick Actions Panel**
**Bot**: Grid de botones con contenedores
**Mini App**:
- Sidebar con lista de contenedores running
- Botones de acción rápida: Stop, Restart, Logs, Terminal
- Drag & drop para reordenar favoritos

### `/stats` → **System Dashboard**
**Bot**: Texto con stats del sistema
**Mini App**:
- Hero section con métricas principales
- Gráficos de línea: CPU/RAM/Disk últimas 24h
- Tabla de top 5 contenedores por recurso
- Alertas activas destacadas

### `/uptime` → **Uptime Widget**
**Bot**: Texto con uptime
**Mini App**:
- Widget en dashboard principal
- Timeline visual de reinicios
- Comparativa con uptime promedio

---

## 🎛️ Container Management Commands

### `/create` → **Container Creation Wizard**
**Bot**: Wizard paso a paso con mensajes
**Mini App**:
- Formulario multi-step con validación en tiempo real
- **Step 1**: Método (Docker Run / Compose)
- **Step 2**: Image selector con búsqueda en Docker Hub
- **Step 3**: Port mapping con detección de conflictos
- **Step 4**: Volume mounting con file browser
- **Step 5**: Environment variables (key-value editor)
- **Step 6**: Network selection con diagrama
- **Preview**: YAML/comando generado antes de crear
- **Templates**: Cargar desde plantilla guardada

### `/restart` → **Action Button**
**Bot**: Grid de contenedores
**Mini App**:
- Botón en Container Detail View
- Confirmación con modal
- Progress indicator
- Notificación de éxito/error

### `/stop`, `/start_container`, `/pause`, `/unpause` → **Action Buttons**
**Bot**: Grids separados
**Mini App**:
- Botones contextuales según estado del contenedor
- Bulk actions: Checkbox múltiple + botón flotante
- Keyboard shortcuts: S (stop), R (restart), P (pause)

### `/logs` → **Logs Viewer**
**Bot**: Texto plano con scroll
**Mini App**:
- Terminal-style viewer con syntax highlighting
- Filtros: Error, Warning, Info, Debug
- Búsqueda con regex
- Auto-scroll toggle
- Download como .log
- Multi-container: Tabs o split view

### `/logfile` → **Download Button**
**Bot**: Envía archivo .log
**Mini App**:
- Botón "Download Logs" en Logs Viewer
- Date range picker
- Formato: .log, .txt, .json

### `/exec` → **Web Terminal**
**Bot**: Input de comando → output
**Mini App**:
- Terminal interactivo (xterm.js)
- Historial de comandos (↑↓)
- Autocompletado básico
- Copy/paste habilitado

### `/env` → **Environment Variables Panel**
**Bot**: Lista de variables
**Mini App**:
- Tabla editable: Key | Value | Actions
- Add/Edit/Delete con validación
- Búsqueda y filtrado
- Export como .env
- Masked values para secrets

### `/inspect` → **Container Detail View**
**Bot**: JSON formateado
**Mini App**:
- Tabs: Overview, Config, Networks, Volumes, Logs, Stats
- JSON viewer con syntax highlighting y collapse
- Copy to clipboard por sección

---

## 🐳 Docker Compose Commands

### `/compose` → **Compose Projects Manager**
**Bot**: Lista de proyectos → acciones
**Mini App**:
- Lista de proyectos detectados
- Por proyecto: Services list con estados
- Actions: Up, Down, Restart, Pull, Logs
- YAML editor con syntax highlighting
- Validación de sintaxis en tiempo real
- Git integration: Commit changes

---

## 🖼️ Images & Updates Commands

### `/images` → **Images Library**
**Bot**: Lista con botones
**Mini App**:
- Grid de cards con imagen, tag, size, created
- Filtros: Used / Unused, por repositorio
- Búsqueda
- Actions: Pull, Remove, Inspect
- Dangling images destacadas

### `/checkupdates` → **Update Center**
**Bot**: Lista de updates disponibles
**Mini App**:
- Dashboard de updates
- Por imagen: Current version → New version
- Changelog preview (si disponible)
- Bulk update con preview
- Schedule updates (fecha/hora)

### `/autoupdate` → **Auto-Update Settings**
**Bot**: Toggle por contenedor
**Mini App**:
- Toggle switches por contenedor
- Configuración avanzada:
  - Check interval
  - Auto-restart
  - Notification preferences
- Historial de auto-updates

### `/trackimage` → **Image Tracker**
**Bot**: Input de imagen
**Mini App**:
- Formulario: Registry, Image, Tag
- Lista de imágenes trackeadas
- Status: Up to date / Update available
- Notifications settings

### `/trackchart` → **Helm Chart Tracker**
**Bot**: Input de chart URL
**Mini App**:
- Búsqueda en Artifact Hub integrada
- Lista de charts trackeados
- Changelog viewer
- One-click deploy (si aplica)

---

## 📦 Resources Commands

### `/volumes` → **Volumes Manager**
**Bot**: Lista con botones
**Mini App**:
- Tabla: Name, Driver, Mountpoint, Size, Containers
- Filtros: In use / Unused
- Actions: Inspect, Remove, Backup
- Visual: Pie chart de uso de espacio

### `/networks` → **Network Visualizer**
**Bot**: Lista de redes
**Mini App**:
- Diagrama interactivo (D3.js o similar)
- Nodos: Contenedores
- Edges: Conexiones de red
- Click en nodo → Container detail
- Create/Remove networks con wizard
- DNS resolution tester

### `/prune` → **Cleanup Wizard**
**Bot**: Confirmación → ejecuta
**Mini App**:
- Checklist: Containers, Images, Volumes, Networks
- Preview de lo que se eliminará
- Estimación de espacio a liberar
- Dry-run mode
- Confirmación con password/PIN

---

## 🔧 Utilities Commands

### `/diagnose` → **Health Dashboard**
**Bot**: Lista de problemas detectados
**Mini App**:
- Health score (0-100)
- Categorías: Containers, Resources, Security
- Por problema: Descripción, Severity, Fix suggestion
- One-click fixes cuando sea posible

### `/search` → **Global Search**
**Bot**: Input → resultados
**Mini App**:
- Barra de búsqueda global (Ctrl+K)
- Búsqueda en: Containers, Images, Volumes, Networks, Logs
- Resultados agrupados por tipo
- Filtros avanzados

### `/favorites` → **Favorites Sidebar**
**Bot**: Lista de favoritos
**Mini App**:
- Sidebar colapsable con favoritos
- Drag & drop para reordenar
- Star icon en cada contenedor para add/remove

### `/addfav` → **Star Button**
**Bot**: Grid de contenedores
**Mini App**:
- Botón estrella en Container Detail View
- Aparece automáticamente en Favorites Sidebar

### `/history` → **Activity Log**
**Bot**: Lista de comandos ejecutados
**Mini App**:
- Timeline de acciones
- Filtros: User, Action type, Date range
- Re-ejecutar comando desde historial

### `/backup` → **Backup Manager**
**Bot**: Genera backup
**Mini App**:
- Scheduled backups configuration
- Manual backup con selección de qué incluir
- Restore from backup con preview
- Backup history con download

### `/version` → **About Section**
**Bot**: Versión del bot
**Mini App**:
- Footer con versión
- About modal: Bot version, Docker version, System info
- Check for updates

---

## 🔄 Phase 2 Commands

### `/rollback` → **Rollback Manager**
**Bot**: Lista de contenedores con historial
**Mini App**:
- Por contenedor: Timeline de versiones
- Visual diff entre versiones
- One-click rollback con confirmación
- Rollback múltiple con preview

### `/templates` → **Template Library**
**Bot**: Lista de templates
**Mini App**:
- Grid de cards con preview
- Categorías: Database, Web, Monitoring, etc.
- Template editor con preview
- Share template (export JSON)
- Import from URL/file

### `/maintenance` → **Maintenance Mode Toggle**
**Bot**: Toggle + lista de pausados
**Mini App**:
- Toggle switch prominente
- Lista de contenedores que se pausarán
- Schedule maintenance window
- Notification to users

---

## ⚠️ Phase 1 Commands

### `/alerts` → **Alerts Configuration**
**Bot**: Por contenedor → thresholds
**Mini App**:
- Dashboard de alertas activas
- Configuración por contenedor:
  - CPU threshold (slider)
  - RAM threshold (slider)
  - Disk threshold (slider)
- Alert history con gráficos
- Notification channels: Telegram, Email, Webhook

### `/healthchecks` → **Health Checks Manager**
**Bot**: Por contenedor → config
**Mini App**:
- Lista de health checks configurados
- Por contenedor: HTTP/TCP/Command check
- Test health check en vivo
- Health check history con uptime %

### `/reports` → **Reports Scheduler**
**Bot**: Configurar schedule
**Mini App**:
- Report templates: Daily, Weekly, Monthly
- Customizar qué incluir en reporte
- Preview de reporte
- Send test report
- Report history con download

---

## 🔒 Phase 3 Commands

### `/audit` → **Audit Log Viewer**
**Bot**: Lista de eventos
**Mini App**:
- Tabla con: Timestamp, User, Action, Resource, Result
- Filtros avanzados
- Export como CSV/JSON
- Retention policy configuration

### `/scan` → **Security Scanner**
**Bot**: Escanear imagen → resultados
**Mini App**:
- Scan all images button
- Por imagen: Vulnerabilities list
- Severity badges: Critical, High, Medium, Low
- CVE details con links
- Scan history

### `/webhooks` → **Webhooks Manager**
**Bot**: CRUD de webhooks
**Mini App**:
- Lista de webhooks configurados
- Add webhook: URL, Events, Headers
- Test webhook con payload preview
- Webhook logs (últimas llamadas)

### `/policies` → **Update Policies**
**Bot**: Por contenedor → policy
**Mini App**:
- Global policy + overrides por contenedor
- Configuración:
  - Auto-update: On/Off
  - Schedule: Cron expression builder
  - Conditions: Only patch, Only minor, etc.
- Policy simulation

---

## 🌐 Phase 4 Commands

### `/networks` → **Network Manager** (ya cubierto arriba)

### `/cleanup` → **Intelligent Cleanup**
**Bot**: Ejecuta cleanup
**Mini App**:
- Análisis de recursos huérfanos
- Recomendaciones de limpieza
- Safe to remove vs. Risky
- Cleanup schedule

### `/ports` → **Port Manager**
**Bot**: Lista de puertos
**Mini App**:
- Tabla: Port, Container, Protocol, Status
- Conflict detection con highlight
- Port availability checker
- Suggest available ports

---

## 🎨 UI/UX Principles

### Navigation
- **Sidebar**: Main sections (Dashboard, Containers, Images, Networks, etc.)
- **Top bar**: Search, Notifications, User menu
- **Breadcrumbs**: Current location
- **Quick actions**: Floating action button

### Responsive Design
- **Desktop**: Full sidebar + main content
- **Tablet**: Collapsible sidebar
- **Mobile**: Bottom navigation bar

### Real-time Updates
- **WebSocket**: Container status, resource usage
- **Polling fallback**: Si WebSocket falla
- **Optimistic UI**: Acciones instantáneas con rollback si falla

### Keyboard Shortcuts
- `Ctrl+K`: Global search
- `Ctrl+R`: Refresh
- `Ctrl+N`: New container
- `Esc`: Close modal
- `S`: Stop selected
- `R`: Restart selected

### Themes
- **Light mode**: Default
- **Dark mode**: Auto-detect from Telegram theme
- **Sync with Telegram**: Usa colores del tema del usuario

---

## 🚀 Implementation Priority

### MVP (Phase 1)
1. Dashboard
2. Container List + Detail View
3. Logs Viewer
4. Start/Stop/Restart actions
5. Stats monitoring

### Phase 2
6. Container Creation Wizard
7. Compose Manager
8. Update Center
9. Network Visualizer

### Phase 3
10. Templates Library
11. Audit Log
12. Security Scanner
13. Advanced monitoring

### Phase 4
14. Collaboration features
15. Multi-user access
16. Shared templates
17. Team notifications
