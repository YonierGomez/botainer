# 🐳 Botainer

[![GitHub Stars](https://img.shields.io/github/stars/YonierGomez/botainer?style=flat&logo=github&label=Stars)](https://github.com/YonierGomez/botainer/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/YonierGomez/botainer?style=flat&logo=github&label=Forks)](https://github.com/YonierGomez/botainer/network/members)
[![GitHub Issues](https://img.shields.io/github/issues/YonierGomez/botainer?logo=github&label=Issues)](https://github.com/YonierGomez/botainer/issues)
[![GitHub License](https://img.shields.io/github/license/YonierGomez/botainer?logo=opensourceinitiative&label=License)](https://github.com/YonierGomez/botainer/blob/main/LICENSE)
[![Last Commit](https://img.shields.io/github/last-commit/YonierGomez/botainer?logo=github&label=Last%20Commit)](https://github.com/YonierGomez/botainer/commits/main)

### Tecnologías

![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![Alpine Linux](https://img.shields.io/badge/Alpine_Linux-0D597F?logo=alpinelinux&logoColor=white)
![Telegram](https://img.shields.io/badge/Telegram-26A5E4?logo=telegram&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker_Compose-2496ED?logo=docker&logoColor=white)

Bot de Telegram escrito en Go para gestionar Docker desde el móvil. Más de 25 comandos, notificaciones en tiempo real, detección automática de actualizaciones de imágenes y una interfaz con botones interactivos.

---

## Requisitos

- Servidor Linux con Docker y Docker Compose instalados
- Token de bot de Telegram (ver sección siguiente)

---

## 1. Crear el bot en Telegram

1. Abre Telegram y busca [@BotFather](https://t.me/botfather)
2. Envía `/newbot`
3. Elige un nombre para el bot (ej: `Mi Docker Bot`)
4. Elige un username que termine en `bot` (ej: `midocker_bot`)
5. BotFather te entregará un token con este formato:

```
123456789:ABCdefGHIjklMNOpqrsTUVwxyz
```

Guarda ese token, lo necesitarás en el siguiente paso.

> Para obtener tu Telegram User ID (necesario para restringir acceso), envía un mensaje a [@userinfobot](https://t.me/userinfobot).

---

## 2. Instalación

```bash
git clone https://github.com/YonierGomez/botainer.git
cd botainer
cp .env.example .env
```

Edita `.env` con tu token:

```bash
nano .env
```

```env
# Requerido
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz

# Opcional: restringe el acceso a estos User IDs (separados por coma)
# Si se deja vacío, cualquier usuario puede usar el bot
ALLOWED_USERS=123456789,987654321
```

---

## 3. Levantar con Docker

```bash
docker compose up -d --build
```

Verifica que esté corriendo:

```bash
docker logs -f botainer
```

Deberías ver: `Bot iniciado: @tu_bot`

Para detenerlo:

```bash
docker compose down
```

---

## 4. Configuración del docker-compose.yml

```yaml
services:
  botainer:
    build: .
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /home/ubuntu:/workspace:ro
    env_file:
      - .env
    network_mode: host
```

El volumen `/var/run/docker.sock` le da acceso al daemon de Docker del host. El volumen `/workspace` apunta al directorio donde viven tus proyectos Docker Compose (ajústalo según tu servidor).

---

## 5. Comandos disponibles

### Menú y estado

| Comando | Descripción |
|---|---|
| `/start` | Menú principal con botones |
| `/list` | Todos los contenedores con estado (🟢🔴🟡) en un solo mensaje |
| `/ps` | Contenedores corriendo con CPU y RAM |
| `/running` | Todos los contenedores con botones de acción |
| `/stats` | Dashboard del sistema (CPU, RAM, disco) |

### Gestión de contenedores

| Comando | Descripción |
|---|---|
| `/create` | Asistente para crear contenedor (Docker Run o Compose) |
| `/restart` | Reiniciar contenedor |
| `/stop` | Detener contenedor |
| `/start_container` | Iniciar contenedor detenido |
| `/pause` / `/unpause` | Pausar / reanudar contenedor |
| `/exec` | Ejecutar comando dentro de un contenedor |
| `/logs` | Ver logs en tiempo real |
| `/logfile` | Descargar logs como archivo `.log` |
| `/inspect` | Inspeccionar contenedores, imágenes, volúmenes y redes |

### Imágenes y actualizaciones

| Comando | Descripción |
|---|---|
| `/checkupdates` | Buscar actualizaciones de imágenes manualmente |
| `/updateall` | Actualizar todas las imágenes y recrear contenedores |
| `/images` | Listar imágenes locales |

### Docker Compose

| Comando | Descripción |
|---|---|
| `/compose` | Gestionar proyectos Compose (up, down, restart, pull, ps) |

### Recursos

| Comando | Descripción |
|---|---|
| `/volumes` | Listar volúmenes |
| `/networks` | Listar redes |
| `/prune` | Limpiar recursos no usados |
| `/search` | Buscar en contenedores, imágenes y volúmenes |

### Utilidades

| Comando | Descripción |
|---|---|
| `/diagnose` | Diagnóstico automático (contenedores caídos, uso alto de recursos) |
| `/favorites` | Ver contenedores favoritos |
| `/addfav` | Agregar contenedor a favoritos |
| `/env` | Ver variables de entorno de un contenedor |
| `/history` | Historial de comandos ejecutados |

---

## 6. Notificaciones automáticas

Las notificaciones se activan al enviar cualquier mensaje al bot. Recibirás alertas de:

- 🟢 Contenedor iniciado
- 🔴 Contenedor detenido
- 💥 Contenedor caído inesperadamente
- 🔄 Contenedor reiniciado
- ⏸️ Contenedor pausado / ▶️ reanudado
- 🗑️ Contenedor eliminado
- 🆕 Nueva versión de imagen disponible (con botón para actualizar)

### Actualizaciones de imágenes

El bot verifica automáticamente si hay nuevas versiones de las imágenes cada 6 horas (primera verificación a los 5 minutos de arrancar). También puedes lanzarlo manualmente con `/checkupdates` o desde el menú principal.

Cuando detecta una actualización, envía una notificación con botones:

- Si el contenedor pertenece a un proyecto Docker Compose → botón **🔄 Pull & Up: \<proyecto\>** que ejecuta `pull` + `up -d`
- Si es un contenedor standalone → botón **🔄 Recrear: \<nombre\>**

---

## 7. Seguridad

Restringe el acceso agregando tu User ID en `.env`:

```env
ALLOWED_USERS=123456789
```

Recomendaciones adicionales:

- Rota el token periódicamente desde @BotFather (`/revoke`)
- No subas el archivo `.env` al repositorio (ya está en `.gitignore`)
- Usa VPN para acceso remoto al servidor

---

## 8. Actualizar el bot

```bash
cd botainer
git pull
docker compose up -d --build
```

---

## 9. Solución de problemas

**El bot no responde**
```bash
docker ps | grep botainer
docker logs --tail 50 botainer
docker compose restart
```

**Error de permisos de Docker**
```bash
sudo usermod -aG docker $USER
newgrp docker
```

**Los comandos no aparecen en Telegram**

Los comandos se registran automáticamente al iniciar. Si no aparecen, reinicia el bot y espera 1-2 minutos, luego escribe `/` en el chat.

---

## Contribuir

1. Crea una rama desde `main`
2. Haz tus cambios y commitea
3. Pushea la rama y abre un Pull Request

```bash
git checkout -b mi-feature
git add -A && git commit -m "feat: mi cambio"
git push origin mi-feature
```

---

## Apoya el proyecto

Si te resulta útil, considera apoyar el desarrollo:

[![Buy Me A Coffee](https://img.shields.io/badge/Buy_Me_A_Coffee-FFDD00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/yoniergomez)
[![GitHub Sponsors](https://img.shields.io/badge/GitHub_Sponsors-EA4AAA?logo=githubsponsors&logoColor=white)](https://github.com/sponsors/YonierGomez)

---

## Links

- [GitHub](https://github.com/YonierGomez/botainer)
- [Web del autor](https://www.yonier.com)

---

## Licencia

MIT — consulta el archivo [LICENSE](LICENSE).
