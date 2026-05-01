# 🐳 Botainer

![GitHub stars](https://img.shields.io/github/stars/YonierGomez/botainer?style=flat-square)
![GitHub forks](https://img.shields.io/github/forks/YonierGomez/botainer?style=flat-square)
![GitHub issues](https://img.shields.io/github/issues/YonierGomez/botainer?style=flat-square)
![License](https://img.shields.io/github/license/YonierGomez/botainer?style=flat-square)
![Last commit](https://img.shields.io/github/last-commit/YonierGomez/botainer?style=flat-square)

Bot completo de Telegram para gestionar Docker desde tu móvil. Más de 25 comandos, notificaciones en tiempo real, y una interfaz intuitiva con iconos y botones.

🌐 **[Ver Landing Page](https://yoniergomez.github.io/botainer/)**

## ✨ Características Principales

- 🚀 **Rápido** - Escrito en Go, respuestas instantáneas
- 📊 **Monitoreo en tiempo real** - CPU, RAM, healthchecks
- 🔔 **Notificaciones automáticas** - Eventos y actualizaciones
- 🐳 **Docker Compose** - Gestión completa de proyectos
- 🔍 **Búsqueda universal** - Contenedores, imágenes, volúmenes
- ⭐ **Favoritos** - Acceso rápido a contenedores frecuentes
- 🔐 **Seguro** - Autenticación por whitelist
- 🎨 **Interfaz bonita** - Iconos específicos, grid de 2 columnas
- 📜 **Historial** - Registro de comandos ejecutados
- 🔧 **Diagnóstico** - Detección automática de problemas
- 🛠️ **Crear contenedores** - Asistente paso a paso para Docker Run y Compose
- 💾 **Descargar logs** - Exporta logs como archivos .log
- ❌ **Chat limpio** - Botón de cerrar en todos los mensajes

Ver [FEATURES.md](FEATURES.md) para lista completa de funcionalidades.

## 📋 Requisitos

- Docker y Docker Compose instalados
- Token de bot de Telegram (obtenerlo de [@BotFather](https://t.me/botfather))
- Servidor Linux (Ubuntu, Debian, etc.)

## 🚀 Instalación Rápida

### 1. Clonar el repositorio

```bash
git clone https://github.com/YonierGomez/botainer.git
cd botainer
```

### 2. Configurar variables de entorno

```bash
cp .env.example .env
nano .env
```

Configura al menos el token:

```env
TELEGRAM_BOT_TOKEN=tu_token_aqui
ALLOWED_USERS=123456,789012  # Opcional
```

### 3. Iniciar el bot

```bash
docker compose up -d --build
```

### 4. Verificar que funciona

```bash
docker logs -f botainer
```

Deberías ver: `Bot iniciado: @tu_bot`

## 🔑 Obtener Token de Telegram

1. Abre Telegram y busca [@BotFather](https://t.me/botfather)
2. Envía `/newbot`
3. Sigue las instrucciones para nombrar tu bot
4. Copia el token que te proporciona
5. Pégalo en `.env`

## 📱 Uso Básico

### Comandos Principales

```
/start       - Menú principal con botones
/create      - Crear nuevo contenedor (Docker Run o Compose)
/ps          - Contenedores corriendo (CPU/RAM)
/stats       - Dashboard del sistema
/compose     - Gestionar proyectos Docker Compose
/search      - Buscar contenedores/imágenes
/diagnose    - Diagnóstico automático
```

### Gestión de Contenedores

```
/restart     - Reiniciar contenedor
/stop        - Detener contenedor
/pause       - Pausar contenedor
/logs        - Ver logs (con filtros)
/logfile     - Descargar logs como archivo .log
/inspect     - Inspeccionar recursos
/exec        - Ejecutar comandos
```

### Recursos

```
/images      - Listar imágenes
/volumes     - Listar volúmenes
/networks    - Listar redes
/prune       - Limpiar recursos no usados
/updateall   - Actualizar todas las imágenes
```

### Utilidades

```
/favorites   - Ver favoritos
/addfav      - Agregar a favoritos
/env         - Ver variables de entorno
/history     - Historial de comandos
```

## 🔔 Notificaciones Automáticas

Las notificaciones se activan automáticamente al enviar cualquier mensaje al bot. No requiere configuración adicional.

Recibirás alertas en tiempo real de:

- 🟢 Contenedor iniciado
- 🔴 Contenedor detenido
- 💥 Contenedor caído inesperadamente
- 🔄 Contenedor reiniciado
- ⏸️ Contenedor pausado / ▶️ reanudado
- 🗑️ Contenedor eliminado

Cada notificación incluye el icono del servicio, el nombre del contenedor y la hora del evento.

## 🔐 Seguridad

### Autenticación por Whitelist

Limita el acceso solo a usuarios autorizados:

```env
ALLOWED_USERS=123456789,987654321
```

Obtén tu User ID de [@userinfobot](https://t.me/userinfobot)

### Recomendaciones

- ✅ Usa whitelist en producción
- ✅ Rota el token periódicamente
- ✅ Ejecuta con usuario sin privilegios
- ✅ Usa VPN para acceso remoto
- ✅ Monitorea los logs regularmente
- ❌ No expongas el token públicamente
- ❌ No uses el bot en redes públicas sin VPN

## 🐳 Docker Compose

El bot se ejecuta como contenedor con acceso al socket de Docker:

```yaml
services:
  botainer:
    build: .
    container_name: botainer
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /home/ubuntu:/workspace
    env_file:
      - .env
```

## 🎨 Iconos de Servicios

El bot reconoce automáticamente más de 40 servicios:

- 🐘 PostgreSQL
- 🐬 MySQL/MariaDB
- 🍃 MongoDB
- 🔴 Redis
- 🟢 Nginx
- 🐍 Python
- ☁️ Nextcloud
- 🎬 Plex/Radarr
- 📺 Sonarr/Jellyfin
- 🔒 Wireguard
- 🛡️ Pi-hole
- Y muchos más...

## 📊 Ejemplos de Uso

### Crear un contenedor

```
/create
→ Seleccionar Docker Run o Compose
→ Seguir el asistente paso a paso
→ Obtener comando o YAML formateado
```

El asistente te pregunta:
- Imagen a usar
- Nombre del contenedor
- Puertos a exponer
- Volúmenes a montar
- Variables de entorno

**Resultado Docker Run:**
```bash
docker run -d --name mi-nginx -p 80:80 -v /data:/usr/share/nginx/html nginx:latest
```

**Resultado Docker Compose:**
```yaml
services:
  web:
    image: nginx:latest
    container_name: web
    restart: unless-stopped
    ports:
      - "80:80"
    volumes:
      - /data:/usr/share/nginx/html
```

### Monitorear contenedores

```
/ps
```

Muestra cada contenedor con:
- Icono específico
- Estado y uptime
- CPU y RAM en tiempo real
- Botones de acción

### Gestionar proyecto Compose

```
/compose
→ Seleccionar proyecto
→ Up / Down / Restart / PS / Pull
```

### Buscar recursos

```
/search nginx
```

Busca en contenedores, imágenes y volúmenes.

### Diagnóstico rápido

```
/diagnose
```

Detecta:
- Contenedores detenidos
- Contenedores unhealthy
- Uso alto de CPU
- Imágenes sin usar

## 🛠️ Desarrollo

### Estructura del Proyecto

```
botainer/
├── main.go              # Código principal del bot
├── Dockerfile           # Imagen del bot
├── docker-compose.yml   # Configuración de despliegue
├── .env                 # Variables de entorno
├── .env.example         # Plantilla de configuración
├── README.md            # Este archivo
├── FEATURES.md          # Documentación completa
└── bot-commands.txt     # Lista de comandos
```

### Compilar localmente

```bash
go build -o botainer main.go
./botainer
```

### Logs

```bash
# Ver logs en tiempo real
docker logs -f botainer

# Últimas 50 líneas
docker logs --tail 50 botainer

# Buscar errores
docker logs botainer 2>&1 | grep -i error
```

## 🔄 Actualización

```bash
cd botainer
git pull
docker compose up -d --build
```

El bot actualiza automáticamente su lista de comandos al iniciar.

## 🐛 Solución de Problemas

### El bot no responde

```bash
# Verificar que está corriendo
docker ps | grep botainer

# Ver logs
docker logs --tail 50 botainer

# Reiniciar
docker compose restart
```

### Error de permisos de Docker

```bash
# Agregar usuario al grupo docker
sudo usermod -aG docker $USER
newgrp docker
```

### Comandos no aparecen en Telegram

Los comandos se configuran automáticamente. Si no aparecen:
1. Reinicia el bot: `docker compose restart`
2. Espera 1-2 minutos
3. Escribe `/` en el chat para ver la lista

## 📈 Rendimiento

- **Imagen Docker**: ~100MB
- **Uso de RAM**: ~20-30MB
- **Tiempo de respuesta**: <1 segundo
- **Lenguaje**: Go (compilado, muy rápido)
- **Concurrencia**: Soporta múltiples usuarios simultáneos

## 🤝 Contribuciones

Las contribuciones son bienvenidas:

1. Fork el proyecto
2. Crea una rama (`git checkout -b feature/nueva-funcionalidad`)
3. Commit tus cambios (`git commit -m 'feat: agregar X'`)
4. Push a la rama (`git push origin feature/nueva-funcionalidad`)
5. Abre un Pull Request

## 📄 Licencia

Este proyecto está bajo la licencia MIT. Consulta el archivo `LICENSE` para más detalles.

## 📞 Soporte

- 🐛 [Issues](https://github.com/YonierGomez/botainer/issues)
- 💬 [Discussions](https://github.com/YonierGomez/botainer/discussions)
- 📧 Email: [tu-email]

## ⭐ Agradecimientos

Si te gusta el proyecto, dale una estrella ⭐ en GitHub!

---

Hecho con ❤️ para la comunidad Docker

## 📋 Requisitos

- Python 3.10 o superior
- Docker y Docker Compose instalados
- Token de bot de Telegram (obtenerlo de [@BotFather](https://t.me/botfather))
- Servidor con acceso SSH (opcional, para ejecución remota)

## 🚀 Instalación

### 1. Clonar el repositorio

```bash
git clone https://github.com/YonierGomez/botainer.git
cd botainer
```

### 2. Crear entorno virtual

```bash
python3 -m venv venv
source venv/bin/activate  # En Windows: venv\Scripts\activate
```

### 3. Instalar dependencias

```bash
pip install -r requirements.txt
```

### 4. Configurar el bot

Copia el archivo de ejemplo y edítalo con tus datos:

```bash
cp .env.example .env
nano .env  # o usa tu editor preferido
```

Configura las siguientes variables:

```env
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
COMPOSE_PROJECT=work_pro
COMPOSE_FILE=docker-compose.yml
```

#### Cómo obtener el token de Telegram:

1. Abre Telegram y busca [@BotFather](https://t.me/botfather)
2. Envía el comando `/newbot`
3. Sigue las instrucciones para nombrar tu bot
4. Copia el token que te proporciona
5. Pégalo en el archivo `.env`

## ▶️ Uso

### Ejecutar el bot

```bash
source venv/bin/activate
python3 docker_telegram_bot.py
```

### Ejecutar en segundo plano (recomendado)

```bash
nohup python3 docker_telegram_bot.py > bot.log 2>&1 &
```

### Ejecutar como servicio systemd (Linux)

Crea el archivo `/etc/systemd/system/botainer.service`:

```ini
[Unit]
Description=Botainer - Telegram Docker Bot
After=network.target docker.service

[Service]
Type=simple
User=tu_usuario
WorkingDirectory=/ruta/a/botainer
Environment="PATH=/ruta/a/botainer/venv/bin"
ExecStart=/ruta/a/botainer/venv/bin/python3 docker_telegram_bot.py
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Luego activa y ejecuta el servicio:

```bash
sudo systemctl daemon-reload
sudo systemctl enable botainer
sudo systemctl start botainer
sudo systemctl status botainer
```

## 📱 Comandos disponibles

Una vez que el bot esté ejecutándose, abre Telegram y busca tu bot:

### Comandos de texto

- `/start` - Menú principal con botones interactivos
- `/pull` - Descargar nuevas versiones de imágenes
- `/up` - Iniciar servicios
- `/list` - Listar todos los servicios definidos en el compose
- `/ps` - Ver estado de todos los contenedores del proyecto
- `/running` - Ver TODOS los contenedores corriendo en el servidor
- `/restart [servicio]` - Reiniciar todos los servicios o uno específico
- `/logs [servicio]` - Ver logs (últimas 100 líneas)
- `/exec <comando>` - Ejecutar comando personalizado de Docker Compose

### Ejemplos de uso

```
/restart nginx          # Reinicia solo el servicio nginx
/logs web               # Muestra logs del servicio web
/exec ps -a             # Ejecuta: docker compose -p work_pro ps -a
```

## 🎛️ Botones del menú

Al enviar `/start`, aparecerá un menú con botones:

- **🔄 Pull & Up** - Actualiza imágenes e inicia servicios (`pull && up -d`)
- **📋 Estado** - Muestra contenedores activos (`ps`)
- **📝 Listar** - Lista todos los servicios definidos (`config --services`)
- **📊 Logs** - Últimas 50 líneas de logs (`logs --tail=50`)
- **🔄 Restart** - Reinicia todos los servicios (`restart`)
- **⏸️ Detener** - Detiene contenedores sin eliminarlos (`stop`)
- **▶️ Iniciar** - Inicia contenedores detenidos (`start`)
- **🗑️ Eliminar** - Elimina contenedores (`down`)
- **🔍 Imágenes** - Lista imágenes del proyecto (`images`)
- **💾 Volúmenes** - Lista volúmenes Docker (`volume ls`)
- **🌐 Redes** - Lista redes Docker (`network ls`)

## 🔒 Seguridad

⚠️ **IMPORTANTE**: Este bot ejecuta comandos en tu servidor. Sigue estas recomendaciones:

### Restricciones recomendadas

1. **Limita el acceso por usuario de Telegram**
   
   Modifica `docker_telegram_bot.py` para agregar validación de usuario:

   ```python
   ALLOWED_USERS = [123456789]  # Tu Telegram user ID
   
   async def check_user(update: Update):
       if update.effective_user.id not in ALLOWED_USERS:
           await update.message.reply_text("❌ No autorizado")
           return False
       return True
   ```

2. **No expongas el token públicamente**
   - Nunca subas el archivo `.env` al repositorio
   - Usa variables de entorno en producción
   - Rota el token periódicamente desde @BotFather

3. **Ejecuta en un entorno controlado**
   - Usa un usuario sin privilegios de root
   - Limita los permisos de Docker al usuario
   - Considera usar Docker en modo rootless

4. **Firewall y red**
   - Restringe el acceso SSH solo a IPs conocidas
   - Usa VPN para acceso remoto
   - Monitorea los logs del bot regularmente

### Obtener tu Telegram User ID

Envía un mensaje a [@userinfobot](https://t.me/userinfobot) en Telegram para obtener tu ID.

## 🛠️ Configuración avanzada

### Cambiar el proyecto de Docker Compose

Edita el archivo `.env`:

```env
COMPOSE_PROJECT=mi_proyecto
COMPOSE_FILE=/ruta/completa/docker-compose.yml
```

### Múltiples proyectos

Puedes ejecutar múltiples instancias del bot con diferentes tokens y proyectos:

```bash
# Bot 1 - Proyecto A
TELEGRAM_BOT_TOKEN=token1 COMPOSE_PROJECT=proyecto_a python3 docker_telegram_bot.py &

# Bot 2 - Proyecto B
TELEGRAM_BOT_TOKEN=token2 COMPOSE_PROJECT=proyecto_b python3 docker_telegram_bot.py &
```

## 🐛 Solución de problemas

### El bot no responde

```bash
# Verifica que el bot esté ejecutándose
ps aux | grep docker_telegram_bot.py

# Revisa los logs
tail -f bot.log
```

### Error de permisos de Docker

```bash
# Agrega tu usuario al grupo docker
sudo usermod -aG docker $USER
newgrp docker
```

### Error de conexión

- Verifica tu conexión a internet
- Confirma que el token sea correcto
- Revisa que Telegram no esté bloqueado en tu red

## 📄 Licencia

Este proyecto está bajo la licencia MIT. Consulta el archivo `LICENSE` para más detalles.

## 🤝 Contribuciones

Las contribuciones son bienvenidas. Por favor:

1. Fork el proyecto
2. Crea una rama para tu feature (`git checkout -b feat/nueva-funcionalidad`)
3. Commit tus cambios (`git commit -m 'feat: agregar nueva funcionalidad'`)
4. Push a la rama (`git push origin feat/nueva-funcionalidad`)
5. Abre un Pull Request

## 📞 Soporte

Si encuentras algún problema o tienes sugerencias:

- Abre un [issue](https://github.com/YonierGomez/botainer/issues)
- Contribuye con un [pull request](https://github.com/YonierGomez/botainer/pulls)

---

Hecho con ❤️ para la comunidad Docker
