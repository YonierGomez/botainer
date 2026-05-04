# Configuración de Notificaciones Automáticas al Canal

Este documento explica cómo configurar las notificaciones automáticas al canal de Telegram cuando se publica una nueva versión.

## 📋 Requisitos

1. Canal de Telegram creado: https://t.me/botainer_news
2. Bot agregado como administrador del canal
3. Secretos configurados en GitHub

---

## 🔧 Paso 1: Agregar el bot como administrador del canal

1. Abre el canal: https://t.me/botainer_news
2. Toca el nombre del canal (arriba)
3. Toca "Administradores"
4. Toca "Agregar administrador"
5. Busca: `@botainerbot`
6. Selecciona el bot
7. **Permisos necesarios:**
   - ✅ Publicar mensajes
   - ❌ Editar mensajes (opcional)
   - ❌ Eliminar mensajes (opcional)
   - ❌ Agregar suscriptores (no necesario)

---

## 🔑 Paso 2: Obtener el Chat ID del canal

Ejecuta este comando en tu servidor:

```bash
# Envía un mensaje de prueba al canal desde el bot
docker exec -it botainer sh -c 'echo "Test" | nc -l -p 8080'
```

**O usa este método más simple:**

1. Reenvía cualquier mensaje del canal a: `@userinfobot`
2. El bot te responderá con el Chat ID del canal
3. El formato será: `-100XXXXXXXXXX` (número negativo)

**O usa la API de Telegram:**

```bash
# Reemplaza YOUR_BOT_TOKEN con el token real
curl -s "https://api.telegram.org/botYOUR_BOT_TOKEN/getUpdates" | grep -o '"chat":{"id":-[0-9]*' | head -1
```

---

## 🔐 Paso 3: Configurar secretos en GitHub

1. Ve a: https://github.com/YonierGomez/botainer/settings/secrets/actions
2. Agrega estos secretos:

### `TELEGRAM_BOT_TOKEN`
- **Valor:** El token del bot (ya lo tienes en `.env`)
- **Ejemplo:** `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`

### `TELEGRAM_CHANNEL_ID`
- **Valor:** El Chat ID del canal (número negativo)
- **Ejemplo:** `-1001234567890`

---

## ✅ Paso 4: Verificar configuración

Una vez configurados los secretos, el workflow enviará automáticamente una notificación al canal cuando:

1. Se detecte una actualización de Alpine, Go o telegram-bot-api
2. Se haga un push a `main` con cambios en el código
3. Se ejecute manualmente con `force_build: true`

**El mensaje incluirá:**
- 🚀 Versión nueva
- 📝 Changelog (qué cambió)
- 📦 Comando para actualizar
- 🔗 Link al release en GitHub

---

## 📝 Ejemplo de notificación

```
🚀 Botainer v3.21.3-go1.24.2-tgapiv5.5.1

🏔️ Alpine Linux updated to 3.21.3
🐹 Go updated to 1.24.2

📦 docker pull yoniergomez/botainer:latest

🔗 Ver cambios
```

---

## 🧪 Probar la notificación

Para probar sin esperar a un release real:

1. Ve a: https://github.com/YonierGomez/botainer/actions
2. Selecciona "Docker Multi-Arch CI"
3. Toca "Run workflow"
4. Selecciona `force_build: true`
5. Toca "Run workflow"

Esto creará un release y enviará la notificación al canal.

---

## 🔍 Troubleshooting

### El bot no puede enviar mensajes al canal

**Error:** `Chat not found` o `Forbidden`

**Solución:**
1. Verifica que el bot sea administrador del canal
2. Verifica que el Chat ID sea correcto (debe ser negativo)
3. Verifica que el bot tenga permiso para "Publicar mensajes"

### El workflow falla en el paso de notificación

**Error:** `Bad Request: chat not found`

**Solución:**
1. El Chat ID está mal configurado
2. Usa el método de `@userinfobot` para obtener el ID correcto

### No se envía notificación

**Posibles causas:**
1. Los secretos no están configurados en GitHub
2. El release ya existía (no se crea uno nuevo)
3. El workflow detectó que no hay cambios (`should_build: false`)

---

## 📚 Referencias

- [Telegram Bot API - sendMessage](https://core.telegram.org/bots/api#sendmessage)
- [GitHub Actions - Secrets](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [Botainer News Channel](https://t.me/botainer_news)
