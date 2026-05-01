# Docker Telegram Bot

Bot de Telegram para gestionar Docker Compose desde el móvil.

## Requisitos

- Python 3.8+
- Docker y Docker Compose instalados
- Token de bot de Telegram (obtenerlo de [@BotFather](https://t.me/botfather))

## Instalación

```bash
pip install -r requirements.txt
```

## Configuración

1. Copia `.env.example` a `.env`:
```bash
cp .env.example .env
```

2. Edita `.env` y configura tu token:
```env
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
COMPOSE_PROJECT=work_pro
COMPOSE_FILE=docker-compose.yml
```

## Uso

```bash
python docker_telegram_bot.py
```

## Comandos disponibles

- `/start` - Menú principal con botones
- `/pull` - Descargar nuevas versiones de imágenes
- `/up` - Iniciar servicios
- `/list` - Listar todos los servicios definidos
- `/ps` - Ver estado de todos los contenedores
- `/restart [servicio]` - Reiniciar todos o un servicio específico
- `/logs [servicio]` - Ver logs (últimas 100 líneas)
- `/exec <comando>` - Ejecutar comando personalizado

## Botones del menú

- **🔄 Pull & Up** - Actualiza imágenes e inicia servicios
- **📋 Estado** - Muestra contenedores activos
- **📝 Listar** - Lista todos los servicios definidos
- **📊 Logs** - Últimas 50 líneas de logs
- **🔄 Restart** - Reinicia todos los servicios
- **⏸️ Detener** - Detiene contenedores sin eliminarlos
- **▶️ Iniciar** - Inicia contenedores detenidos
- **🗑️ Eliminar** - Elimina contenedores
- **🔍 Imágenes** - Lista imágenes del proyecto
- **💾 Volúmenes** - Lista volúmenes Docker
- **🌐 Redes** - Lista redes Docker

## Seguridad

⚠️ Este bot ejecuta comandos en tu servidor. Recomendaciones:

1. Restringe el acceso solo a tu usuario de Telegram
2. No expongas el token públicamente
3. Ejecuta en un entorno controlado
