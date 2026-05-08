# Changelog v1.2.1

## 🎯 Nueva Funcionalidad Principal

### Detección Inteligente de Tags Más Nuevos
- **Detecta versiones semver más recientes** automáticamente (ej: `alpine:3.18` → `alpine:3.23` disponible)
- **Actualización con un click** para servicios compose y contenedores standalone
- **Edición automática de compose.yaml** para servicios compose
- **Recreación inteligente** para contenedores standalone preservando toda la configuración

## ✨ Mejoras de Rendimiento

### Paralelización Completa
- Verificación de imágenes en paralelo (10 concurrentes)
- Timeout de 10 segundos por verificación de tag
- Semáforo para controlar concurrencia
- Reducción de tiempo: ~60s → ~15-20s

### Optimización de Verificaciones
- Solo verifica cada imagen única una vez
- Agrupa contenedores que usan la misma imagen
- Salta tags flotantes automáticamente (latest, alpine, stable)
- Cache de tokens de registry

## 🎨 Mejoras de UX

### Notificaciones Mejoradas
- Muestra todos los contenedores que usan la misma imagen
- Botón de actualización individual por contenedor
- Contador de progreso: "🔄 Verificando X contenedores..."
- Mensajes más claros y descriptivos

### Botones de Acción
- **🔄 Actualizar: \<container\>** - Actualiza ese contenedor específico
- **❌ Cerrar** - Cierra la notificación
- Múltiples botones cuando varios contenedores usan la misma imagen

## 🔧 Cambios Técnicos

### Eliminaciones
- ❌ Comando `/updateall` eliminado (redundante)
- ✅ `/checkupdates` ahora hace TODO (digest + tags más nuevos)

### Nuevas Funciones
- `findNewerTag()` - Busca tags más nuevos en registry
- `tagParts()` - Extrae versión y sufijo de tags
- `parseRegistryAndRepo()` - Parsea imagen para obtener registry y repo
- `fetchRegistryToken()` - Obtiene token de autenticación
- `listRegistryTags()` - Lista todos los tags disponibles

### Handler de Actualización
- `newtag_update` callback handler
- Edita compose.yaml con `sed` para servicios compose
- Ejecuta `docker compose up -d <service>` automáticamente
- Recrea contenedores standalone con nueva imagen

## 📊 Estadísticas

- **Commits**: 15+ commits
- **Líneas agregadas**: ~300 líneas
- **Líneas eliminadas**: ~150 líneas (eliminación de /updateall)
- **Archivos modificados**: main.go, README.md, docs/index.html, locale/*.json

## 🐛 Fixes

- Corregido error de compilación con `NetworkingConfig`
- Eliminadas referencias huérfanas a `handleUpdateAll`
- Agregada declaración de variable `out` antes de uso
- Corregido timeout en verificaciones de tags

## 📚 Documentación

- README actualizado con nuevas capacidades
- Landing page actualizada con feature card
- Eliminado `/updateall` de lista de comandos
- Descripciones mejoradas en meta tags

## 🚀 Próximos Pasos Sugeridos

1. Monitorear rendimiento en producción
2. Considerar agregar botón "Actualizar todos" para múltiples contenedores
3. Agregar opción de rollback si la actualización falla
4. Implementar notificaciones de éxito/error post-actualización
5. Considerar agregar preview de cambios antes de actualizar

---

**Versión**: 1.2.1  
**Fecha**: 2026-05-05  
**Commits**: a9c291c → 941c2bf
