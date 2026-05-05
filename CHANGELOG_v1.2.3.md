# Changelog v1.2.3

## 🎯 Nueva Funcionalidad Principal

### Tracking de Helm Charts con Imágenes de Contenedor
- **Monitoreo completo de Helm charts** desde Artifact Hub
- **Muestra imágenes de contenedor** usadas por cada chart
- **Información detallada**: Chart version, app version, repo, y todas las imágenes
- **Notificaciones automáticas** cuando hay nuevas versiones disponibles
- **Formato mejorado** en listado de charts trackeados

## 🔧 Mejoras de Persistencia

### Sistema de Configuración Robusto
- **Volumen dedicado `/data`** para persistencia
- **Estructura `ChartInfo`** para almacenar información completa de charts
- **Migración automática** de configuración antigua
- **Documentación completa** de volúmenes en README

## 📚 Documentación

### README Mejorado
- **README_EN.md** creado con documentación completa en inglés
- **Tabla de volúmenes** con descripción de cada uno
- **Ejemplos actualizados** de docker-compose.yml con volumen `botainer_data`
- **Sección de persistencia** expandida con detalles de qué se guarda

### Locale
- **Archivo en.json** verificado y actualizado
- **Soporte completo** para inglés y español

## 🐛 Fixes

### Persistencia
- **Fix**: Configuración se perdía al reiniciar el bot
- **Solución**: Volumen `/data` documentado y requerido en compose
- **Mejora**: Directorio `/data` se crea automáticamente si no existe

### Estructura de Datos
- **ChartInfo** ahora incluye array de imágenes
- **Almacenamiento completo** de metadata de charts
- **Actualización automática** de información al verificar updates

## 📊 Cambios Técnicos

### Nuevas Estructuras
```go
type ChartInfo struct {
    Version    string   `json:"version"`
    AppVersion string   `json:"appVersion"`
    Repo       string   `json:"repo"`
    Images     []string `json:"images"`
}
```

### API de Artifact Hub
- **Campo `containers_images`** parseado correctamente
- **Extracción de imágenes** de cada chart
- **Almacenamiento persistente** de imágenes

### Mejoras en Listado
- **Formato mejorado** con información completa
- **Listado de imágenes** por cada chart
- **Mejor legibilidad** con formato multi-línea

## 📦 Archivos Modificados

- `main.go` - Estructura ChartInfo, persistencia, listado mejorado
- `README.md` - Tabla de volúmenes, ejemplos actualizados
- `README_EN.md` - Documentación completa en inglés
- `locale/en.json` - Verificado y actualizado

## 🚀 Próximos Pasos Sugeridos

1. Agregar botón para ver detalles del chart en Artifact Hub
2. Implementar tracking de charts desde repositorios privados
3. Agregar comparación de versiones de imágenes entre charts
4. Notificar cuando una imagen específica del chart se actualiza
5. Implementar búsqueda de charts desde el bot

---

**Versión**: 1.2.3  
**Fecha**: 2026-05-05  
**Tipo**: Fix (persistencia) + Feature (Helm chart images)
