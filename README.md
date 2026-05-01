# 🐳 Botainer

![GitHub stars](https://img.shields.io/github/stars/YonierGomez/botainer?style=flat-square)
![GitHub forks](https://img.shields.io/github/forks/YonierGomez/botainer?style=flat-square)
![GitHub issues](https://img.shields.io/github/issues/YonierGomez/botainer?style=flat-square)
![License](https://img.shields.io/github/license/YonierGomez/botainer?style=flat-square)
![Last commit](https://img.shields.io/github/last-commit/YonierGomez/botainer?style=flat-square)

Bot de Telegram para gestionar Docker Compose desde tu móvil. Controla tus contenedores, actualiza imágenes, revisa logs y más, todo desde la comodidad de Telegram.

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
