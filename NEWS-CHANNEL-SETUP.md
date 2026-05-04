# 📢 Sistema de Notificaciones de Versiones - Botainer

## Cómo Funciona

Botainer ahora incluye un sistema de notificaciones para mantener a los usuarios informados sobre nuevas versiones y características.

---

## 🎯 Características Implementadas

### 1. **Versión del Bot**
- Constante `botVersion` en el código: `2.0.0`
- Se muestra al ejecutar `/start` o `/version`

### 2. **Canal de Telegram**
- URL configurada: `https://t.me/botainer_news`
- Los usuarios pueden suscribirse para recibir actualizaciones

### 3. **Comando `/version`**
Muestra:
```
🤖 Botainer v2.0.0

📢 Mantente al día con las últimas novedades y actualizaciones:

[📢 Canal de Novedades] [⭐ GitHub]
[❌ Cerrar]
```

### 4. **Notificación al Iniciar**
Cuando un usuario ejecuta `/start`, automáticamente ve:
- Versión actual del bot
- Link al canal de novedades
- Link al repositorio de GitHub

---

## 📝 Cómo Configurar el Canal de Telegram

### Paso 1: Crear el Canal
1. Abre Telegram
2. Menú → Nuevo Canal
3. Nombre: `Botainer News` (o el que prefieras)
4. Descripción: `Novedades y actualizaciones de Botainer`
5. Tipo: **Público**
6. Username: `botainer_news` (o el que prefieras)

### Paso 2: Configurar el Canal
1. Agrega una foto de perfil (usa `docs/botainer-image.png`)
2. Configura descripción completa:
   ```
   📢 Canal oficial de novedades de Botainer
   
   🤖 Bot de Telegram para gestionar Docker
   🐳 Controla contenedores desde tu móvil
   ⚡ Actualizaciones, nuevas features y más
   
   🔗 GitHub: https://github.com/YonierGomez/botainer
   📖 Docs: https://yoniergomez.github.io/botainer/
   ```

### Paso 3: Publicar Primera Actualización
```
🎉 ¡Bienvenido al canal de Botainer!

📢 Aquí publicaremos:
✅ Nuevas versiones
✅ Características implementadas
✅ Correcciones importantes
✅ Guías y tutoriales

🤖 Versión actual: v2.0.0
🚀 Migración completa a Docker SDK API

📖 Más info: https://github.com/YonierGomez/botainer
```

### Paso 4: Actualizar el Código (Ya hecho)
El código ya está configurado con:
```go
const (
    botVersion    = "2.0.0"
    newsChannelURL = "https://t.me/botainer_news"
)
```

---

## 📋 Plantilla para Anunciar Nuevas Versiones

### Versión Mayor (v3.0.0)
```
🎉 Botainer v3.0.0 disponible!

🆕 Nuevas características:
• [Feature 1]
• [Feature 2]
• [Feature 3]

⚡ Mejoras:
• [Improvement 1]
• [Improvement 2]

🐛 Correcciones:
• [Fix 1]
• [Fix 2]

📖 Changelog completo:
https://github.com/YonierGomez/botainer/releases/tag/v3.0.0

🔄 Para actualizar:
docker compose pull && docker compose up -d --build botainer
```

### Versión Menor (v2.1.0)
```
✨ Botainer v2.1.0

🆕 Nuevas características:
• [Feature]

⚡ Mejoras:
• [Improvement]

📖 Más info:
https://github.com/YonierGomez/botainer/releases/tag/v2.1.0
```

### Parche (v2.0.1)
```
🔧 Botainer v2.0.1

🐛 Correcciones:
• [Fix 1]
• [Fix 2]

📖 Detalles:
https://github.com/YonierGomez/botainer/releases/tag/v2.0.1
```

---

## 🔄 Flujo de Actualización

### Para el Desarrollador (Tú)
1. **Desarrollar nueva versión**
2. **Actualizar `botVersion` en `main.go`**
3. **Crear commit y tag:**
   ```bash
   git commit -m "feat: nueva característica"
   git tag v2.1.0
   git push origin main --tags
   ```
4. **Crear GitHub Release** con changelog
5. **Publicar en canal de Telegram** usando plantilla
6. **Actualizar Docker Hub** (automático con CI/CD)

### Para los Usuarios
1. **Reciben notificación** en canal de Telegram
2. **Leen changelog** y deciden si actualizar
3. **Actualizan con:**
   ```bash
   docker compose pull
   docker compose up -d --build botainer
   ```
4. **Verifican versión** con `/version`

---

## 🎨 Personalización

### Cambiar URL del Canal
Edita en `main.go`:
```go
const (
    newsChannelURL = "https://t.me/tu_canal"
)
```

### Cambiar Versión
Edita en `main.go`:
```go
const (
    botVersion = "2.1.0"
)
```

### Personalizar Mensaje
Edita la función `checkBotVersion()` en `main.go`

---

## 📊 Estadísticas del Canal

Para ver estadísticas del canal:
1. Abre el canal en Telegram
2. Menú → Estadísticas
3. Verás:
   - Suscriptores
   - Vistas de publicaciones
   - Crecimiento

---

## ✅ Checklist de Lanzamiento

Antes de publicar una nueva versión:
- [ ] Código probado y funcionando
- [ ] `botVersion` actualizado en código
- [ ] Commit y tag creados
- [ ] GitHub Release publicado con changelog
- [ ] Anuncio en canal de Telegram
- [ ] Docker Hub actualizado
- [ ] README actualizado si es necesario

---

## 🔗 Enlaces Útiles

- **Canal de Telegram:** https://t.me/botainer_news
- **GitHub:** https://github.com/YonierGomez/botainer
- **Docker Hub:** https://hub.docker.com/r/yoniergomez/botainer
- **Landing Page:** https://yoniergomez.github.io/botainer/

---

## 💡 Consejos

1. **Publica regularmente** - Mantén a los usuarios informados
2. **Sé claro** - Explica qué cambió y por qué
3. **Incluye ejemplos** - Muestra cómo usar nuevas features
4. **Agradece feedback** - Fomenta la participación
5. **Documenta breaking changes** - Avisa si algo deja de funcionar

---

## 🎯 Próximos Pasos

1. **Crear el canal** `@botainer_news` en Telegram
2. **Publicar mensaje de bienvenida**
3. **Anunciar v2.0.0** (migración a Docker SDK)
4. **Promocionar el canal** en README y landing page
