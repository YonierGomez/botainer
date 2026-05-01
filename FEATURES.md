# 🚀 Características de Botainer

Bot completo de Telegram para gestionar Docker desde tu móvil con más de 25 comandos y funcionalidades avanzadas.

## 📋 Gestión de Contenedores

### Creación de Contenedores
- **`/create`** - **NUEVO** Asistente para crear contenedores
  - Opción Docker Run o Docker Compose
  - Guía paso a paso interactiva
  - Genera comando completo o YAML formateado
  - Opción de ejecutar directamente
  - Soporte para puertos, volúmenes, variables de entorno
  - Comando `/skip` para omitir pasos opcionales

### Visualización y Monitoreo
- **`/ps`** - Lista contenedores corriendo con:
  - Icono específico por servicio (🐘 PostgreSQL, 🐬 MySQL, 🍃 MongoDB, etc.)
  - Estado y uptime
  - Uso de CPU y RAM en tiempo real
  - Proyecto Docker Compose (si aplica)
  - Healthcheck status (💚 healthy, ❤️ unhealthy, 🟡 starting)
  - Botones de acción rápida

- **`/running`** - Todos los contenedores (corriendo y detenidos)
  - Indicador de estado (🟢 corriendo, 🔴 detenido)
  - Botones contextuales según estado

- **`/stats`** - Dashboard del sistema
  - Uso de disco y RAM del host
  - Conteo de recursos Docker
  - Contenedores corriendo vs totales

### Control de Contenedores
- **`/restart`** - Reiniciar contenedores (grid de selección)
- **`/stop`** - Detener contenedores
- **`/start_container`** - Iniciar contenedores detenidos
- **`/pause`** - Pausar contenedores (congela procesos)
- **`/unpause`** - Reanudar contenedores pausados

### Logs Avanzados
- **`/logs`** - Ver logs con características:
  - Resaltado automático de errores (🔴) y warnings (🟡)
  - Filtros por tipo (errors, warnings)
  - Botón para ver más logs
  - Botón de refresh
  - Botón para descargar como archivo .log
  - Formato HTML con syntax highlighting

- **`/logfile`** - **NUEVO** Descargar logs como archivo
  - Exporta últimas 1000 líneas
  - Archivo: `contenedor_timestamp.log`
  - Descarga directa en Telegram

## 🐳 Docker Compose

### `/compose` - Gestión Completa de Proyectos
- Lista automática de proyectos detectados
- Menú por proyecto con:
  - **Up** - Iniciar servicios
  - **Down** - Detener y eliminar
  - **Restart** - Reiniciar servicios
  - **PS** - Ver contenedores del proyecto (formato bonito)
  - **Pull** - Actualizar imágenes

## 🖼️ Gestión de Imágenes

### `/images` - Listar Imágenes
- Nombre, tag y tamaño
- Botones por imagen:
  - 🔍 Inspect - Ver detalles completos
  - 🗑️ Delete - Eliminar imagen
- Sin previews de URLs (ghcr.io, docker.io, etc.)

### `/updateall` - Actualización Inteligente
- Detecta automáticamente tipo de contenedor
- **Proyectos Compose**: `docker compose pull && up -d`
- **Contenedores standalone**: Pull y recrear
- Actualiza todo con un solo comando

## 💾 Volúmenes y Redes

### `/volumes` - Gestión de Volúmenes
- Lista con información de uso
- Muestra qué contenedores usan cada volumen
- Proyecto Docker Compose asociado
- Botones: Inspect y Delete

### `/networks` - Gestión de Redes
- Driver y scope
- Contenedores conectados
- Proyecto asociado
- Botones: Inspect y Delete

## 🔍 Búsqueda e Inspección

### `/search <término>` - Búsqueda Universal
- Busca en contenedores, imágenes y volúmenes
- Resultados agrupados por tipo
- Búsqueda case-insensitive

### `/inspect` - Inspección Detallada
- Menú con opciones:
  - 📦 Contenedores
  - 🖼️ Imágenes
  - 💾 Volúmenes
  - 🌐 Redes
- Muestra JSON completo (truncado si es muy largo)
- Navegación con botones "Atrás"

## ⚙️ Ejecución de Comandos

### `/exec` - Ejecutar en Contenedores
- Selección de contenedor
- Comandos rápidos:
  - 🐚 Shell (sh/bash)
  - 📋 ps aux
  - 📁 ls -la
  - 🌐 netstat
  - 💾 df -h
- Instrucciones para shells interactivos

## 🗑️ Limpieza de Recursos

### `/prune` - Limpiar Sistema
- **Imágenes** - Elimina imágenes sin usar
- **Volúmenes** - Elimina volúmenes sin usar
- **Redes** - Elimina redes sin usar
- **Todo** - Limpieza completa del sistema
- ⚠️ Confirmación antes de ejecutar

## ⭐ Favoritos

### Sistema de Favoritos Personales
- **`/favorites`** - Ver tus contenedores favoritos
- **`/addfav <contenedor>`** - Agregar a favoritos
- Acceso rápido con botones de acción
- Botón para quitar de favoritos
- Persistente por usuario

## 🔧 Variables de Entorno

### `/env` - Ver Variables
- Selección de contenedor
- Lista completa de variables de entorno
- Formato legible
- Truncado automático si es muy largo

## 📜 Historial

### `/history` - Historial de Comandos
- Últimos 50 comandos ejecutados
- Por usuario (no compartido)
- Numerado para referencia

## 🔍 Diagnóstico

### `/diagnose` - Diagnóstico Automático
Detecta y reporta:
- ⚠️ Contenedores detenidos
- ❤️ Contenedores no saludables (unhealthy)
- 🔥 Uso alto de CPU (>80%)
- 🗑️ Imágenes sin usar (dangling)
- Sugerencias de optimización

## 🔔 Notificaciones Automáticas

### Eventos en Tiempo Real
Configura `NOTIFY_CHAT_ID` en `.env` para recibir:
- ▶️ Contenedor iniciado
- ⏸️ Contenedor detenido
- 💀 Contenedor murió (crash)
- 🔄 Contenedor reiniciado
- 🗑️ Contenedor eliminado

### Detección de Actualizaciones
- Revisa cada hora si hay nuevas versiones
- 🆕 Notificación con botón de actualización
- Diferencia entre compose y standalone
- Actualización con un click

## 🔐 Seguridad

### Autenticación por Whitelist
- Variable `ALLOWED_USERS` en `.env`
- Lista de Telegram User IDs permitidos
- Bloqueo automático de usuarios no autorizados
- Logs de intentos no autorizados

## 🎨 Interfaz

### Diseño Intuitivo
- Grid de 2 columnas para selección
- Iconos específicos por servicio (40+ iconos)
- Botones contextuales según estado
- Navegación con botones "Atrás"
- Mensajes formateados con Markdown
- Sin previews de URLs molestas

### Menú Principal (`/start`)
Acceso rápido a:
- 📋 Estado (ps)
- 📊 Stats
- 📁 Compose
- 🔍 Inspect
- 🖼️ Images
- 💾 Volumes
- 🌐 Networks
- ⚙️ Exec
- 🗑️ Prune
- 🔄 Update All

## 🚀 Rendimiento

### Optimizaciones
- Bot escrito en Go (rápido y eficiente)
- Stats en paralelo (no bloquea UI)
- Respuestas instantáneas
- Imagen Docker de ~100MB
- Bajo uso de recursos

### Concurrencia
- Goroutines para operaciones paralelas
- No bloquea en operaciones largas
- Múltiples usuarios simultáneos

## 📊 Comandos Disponibles

Total: **24 comandos**

### Básicos
`start`, `ps`, `running`, `stats`

### Gestión
`restart`, `stop`, `start_container`, `pause`, `unpause`

### Avanzados
`compose`, `exec`, `inspect`, `search`, `env`

### Recursos
`images`, `volumes`, `networks`, `prune`, `updateall`

### Utilidades
`favorites`, `addfav`, `history`, `diagnose`, `logs`

## 🔄 Configuración Automática

- Comandos se registran automáticamente al iniciar
- No requiere configuración manual en BotFather
- Actualización automática de la lista de comandos

## 📝 Logs y Auditoría

- Logs de todos los comandos ejecutados
- Registro de usuarios no autorizados
- Historial por usuario
- Logs estructurados para debugging

## 🌐 Compatibilidad

- Docker Engine 20.10+
- Docker Compose v2
- Alpine Linux (imagen base)
- Telegram Bot API v5

## 💡 Casos de Uso

1. **Administración remota** - Gestiona servidores desde el móvil
2. **Monitoreo 24/7** - Notificaciones de eventos
3. **Troubleshooting** - Diagnóstico y logs rápidos
4. **Actualizaciones** - Update con un click
5. **Equipos** - Whitelist para múltiples usuarios
6. **Homelab** - Control de servicios caseros
7. **Producción** - Monitoreo de servicios críticos

## 🎯 Próximas Características

- Backups automáticos de volúmenes
- Gráficos de uso histórico
- Webhooks para CI/CD
- Integración con Portainer
- Reportes programados
- Alertas personalizables

---

**Versión**: 2.0  
**Última actualización**: Mayo 2026
