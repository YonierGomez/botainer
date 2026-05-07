# 📱 Guía de Usuario - Mini App v2.1

## 🎯 Funcionalidades Implementadas

### 1. 📊 Historical Charts (Gráficos Históricos)

**¿Qué hace?**
Muestra gráficos de tendencias de CPU y memoria de tus contenedores en el tiempo.

**¿Cómo usarlo?**
1. Abre el bot en Telegram: [@botainerbot](https://t.me/botainerbot)
2. Toca el botón **🐳 Dashboard** (o envía `/start` y selecciona Dashboard)
3. Busca un contenedor que esté **corriendo** (🟢)
4. Toca el botón **📈 Charts** del contenedor
5. Selecciona el rango de tiempo:
   - **1h** - Última hora (datos cada 30 segundos)
   - **24h** - Últimas 24 horas
   - **7d** - Últimos 7 días

**¿Qué verás?**
- **Gráfico de CPU**: Porcentaje de uso de CPU en el tiempo
- **Gráfico de Memoria**: Porcentaje de uso de RAM en el tiempo
- Ambos gráficos son interactivos (puedes hacer hover para ver valores exactos)

**Ejemplo de uso**:
```
Contenedor: nginx
Rango: 24h
Resultado: Verás cómo el CPU y la memoria han variado en las últimas 24 horas
```

---

### 2. 📥 Export Metrics (Exportar Métricas)

**¿Qué hace?**
Descarga todas las métricas de todos tus contenedores en formato CSV o JSON.

**¿Cómo usarlo?**
1. Abre el Dashboard en el bot
2. Toca el botón **📥** en la esquina superior derecha (al lado del botón de refresh)
3. Selecciona el rango de tiempo:
   - **1h** - Última hora
   - **24h** - Últimas 24 horas
   - **7d** - Últimos 7 días
   - **30d** - Últimos 30 días
4. Selecciona el formato:
   - **JSON** - Para análisis programático
   - **CSV** - Para abrir en Excel/Google Sheets
5. Toca **📥 Export**
6. El archivo se descargará automáticamente

**Formato CSV**:
```csv
timestamp,container_id,container_name,cpu_percent,memory_usage_gb,memory_limit_gb,memory_percent
1778122348,a0371524852a,/botainer,0.055,0.014,15.583,0.088
1778122350,97f55c82303d,/cloudflare-ddns,0.000,0.024,15.583,0.154
```

**Formato JSON**:
```json
[
  {
    "timestamp": 1778122348,
    "container_id": "a0371524852a",
    "container_name": "/botainer",
    "cpu_percent": 0.055,
    "memory_usage": 0.014,
    "memory_limit": 15.583,
    "memory_percent": 0.088
  }
]
```

**Casos de uso**:
- Análisis de rendimiento histórico
- Reportes para clientes
- Detección de patrones de uso
- Planificación de capacidad
- Importar a herramientas de análisis (Excel, Python, R)

---

## 🔧 Cómo Funciona (Técnico)

### Sistema de Recolección de Métricas

**Backend**:
- El bot recolecta métricas cada **30 segundos** automáticamente
- Almacena hasta **10,080 puntos** (7 días de datos)
- Los datos se guardan en `/data/metrics.json` (persisten entre reinicios)
- Usa la API de Docker para obtener stats en tiempo real

**Endpoints API**:
```
GET /api/containers/{id}/metrics?duration=1h
GET /api/metrics?duration=24h
GET /api/metrics/export?duration=7d&format=csv
```

**Frontend**:
- React 19 + TypeScript
- Recharts para gráficos interactivos
- Auto-refresh de datos
- Responsive (funciona en móvil, tablet, desktop)

---

## ❓ Preguntas Frecuentes

### ¿Por qué no veo datos en los gráficos?

**Posibles causas**:
1. **El contenedor acaba de iniciar**: Espera 1-2 minutos para que se recolecten datos
2. **El bot se reinició recientemente**: Los datos se están recolectando desde cero
3. **El contenedor está detenido**: Solo se muestran gráficos de contenedores corriendo
4. **Caché del navegador**: Cierra y vuelve a abrir el Dashboard

### ¿Cuánto espacio ocupan las métricas?

Aproximadamente **30-50 KB** por cada 7 días de datos (muy ligero).

### ¿Puedo ver métricas de contenedores detenidos?

No, solo se recolectan métricas de contenedores en estado **running**. Si detienes un contenedor, sus métricas históricas se conservan pero no se actualizan.

### ¿Los datos persisten si reinicio el bot?

Sí, las métricas se guardan en `/data/metrics.json` que está en un volumen Docker persistente.

### ¿Puedo cambiar la frecuencia de recolección?

Actualmente está fijada en 30 segundos. Para cambiarla, edita `main.go`:
```go
go api.CollectMetrics(cli, metricsStore, 30*time.Second) // Cambiar 30 por otro valor
```

### ¿Cómo limpio las métricas antiguas?

El sistema automáticamente mantiene solo los últimos 10,080 puntos (7 días). Los datos más antiguos se eliminan automáticamente.

---

## 🐛 Solución de Problemas

### El botón 📈 Charts no aparece

**Solución**: Solo aparece en contenedores que están **corriendo** (🟢). Inicia el contenedor primero.

### El botón 📥 Export no hace nada

**Solución**:
1. Verifica que el bot esté corriendo: `docker logs botainer`
2. Recarga el Dashboard (botón 🔄)
3. Cierra y vuelve a abrir el bot en Telegram

### Los gráficos están vacíos

**Solución**:
1. Espera 1-2 minutos para que se recolecten datos
2. Verifica que el contenedor esté corriendo
3. Cambia el rango de tiempo (prueba con "1h")

### Error "Unauthorized" al exportar

**Solución**: Cierra completamente el bot en Telegram y vuelve a abrirlo. Esto refrescará la autenticación.

---

## 📊 Próximas Funcionalidades (v2.2-v2.3)

### En desarrollo:
- 🚨 **Alerts System**: Notificaciones cuando CPU/RAM superen umbrales
- ✅ **Bulk Operations**: Iniciar/detener múltiples contenedores a la vez
- 🐳 **Docker Compose Manager**: Gestionar proyectos Compose completos
- 🌐 **Network Visualizer**: Ver conexiones entre contenedores
- 👥 **Multi-User Support**: Compartir acceso con tu equipo
- 📚 **Template Library**: Guardar y reutilizar configuraciones

---

## 💡 Tips y Trucos

1. **Monitoreo rápido**: Usa el rango "1h" para ver cambios recientes
2. **Análisis de tendencias**: Usa "7d" para detectar patrones semanales
3. **Reportes mensuales**: Exporta en CSV y abre en Excel para crear gráficos personalizados
4. **Detección de picos**: Los gráficos muestran claramente cuándo hubo picos de uso
5. **Comparación**: Exporta métricas de diferentes períodos y compáralos

---

## 📞 Soporte

- **GitHub Issues**: [github.com/YonierGomez/botainer/issues](https://github.com/YonierGomez/botainer/issues)
- **Telegram Channel**: [@botainer_news](https://t.me/botainer_news)
- **Documentación**: [README.md](README.md)

---

**Versión**: 2.1.0  
**Última actualización**: Mayo 6, 2026
