# Pruebas de Compose - Botainer

## ✅ Validaciones Implementadas

### 1. Validación al Inicio
- ✅ Verifica que `docker compose` esté disponible
- ✅ Muestra versión en logs: `Docker Compose version v2.40.3`
- ✅ Si falla, muestra warning pero no detiene el bot

### 2. Detección de Archivos Compose
Busca en orden:
1. `compose.yaml`
2. `compose.yml`
3. `docker-compose.yaml`
4. `docker-compose.yml`

### 3. Validación de Directorio de Trabajo
- ✅ Verifica que el directorio exista
- ✅ Verifica que el archivo compose exista
- ✅ Mapea correctamente HOST_HOME → WORKSPACE
- ✅ Logs detallados si algo falla

### 4. Timeouts
- **Pull:** 5 minutos (imágenes grandes pueden tardar)
- **Up:** 3 minutos (recreación de contenedores)
- Si excede timeout, muestra error claro

### 5. Manejo de Errores
- ✅ Captura output completo de errores
- ✅ Muestra errores al usuario (truncado si es muy largo)
- ✅ Logs detallados en servidor
- ✅ No crashea si falla

## 🧪 Cómo Probar

### Prueba 1: Actualizar Proyecto Compose
1. Ejecuta `/updateall` en Telegram
2. Si hay actualizaciones en contenedores de compose, verás:
   ```
   🆕 Actualización disponible
   
   📦 Contenedor: botainer
   🖼️ Imagen: work_pro-botainer
   
   [🔄 Pull & Up: work_pro] [❌ Ignorar]
   ```
3. Presiona "Pull & Up: work_pro"
4. Debería mostrar: `✅ Proyecto work_pro actualizado correctamente`

### Prueba 2: Verificar Logs
```bash
docker logs -f botainer
```

Deberías ver:
```
Updating compose project work_pro with file: /workspace/chips_all/compose.yaml
Successfully updated compose project: work_pro
```

### Prueba 3: Error Handling
Para probar manejo de errores, puedes:
1. Temporalmente renombrar el compose.yaml
2. Intentar actualizar
3. Debería mostrar: `❌ No se encontró archivo compose en: ...`

## 📊 Estado Actual

### Contenedores Compose Detectados
```bash
docker ps -a --filter "label=com.docker.compose.project" --format "{{.Names}} ({{.Label \"com.docker.compose.project\"}})"
```

Resultado:
- 33 contenedores con labels de compose
- Proyectos: work_pro, y otros

### Archivos Compose Verificados
- ✅ `/home/ubuntu/chips_all/compose.yaml` existe
- ✅ Accesible desde contenedor en `/workspace/chips_all/compose.yaml`
- ✅ Docker Compose v2.40.3 disponible

## 🔒 Seguridad

### Validaciones de Seguridad
- ✅ Solo usuarios autorizados pueden ejecutar comandos
- ✅ Paths validados antes de ejecutar
- ✅ Timeouts para prevenir comandos colgados
- ✅ No ejecuta comandos arbitrarios (solo compose pull/up)

### Logs de Auditoría
Todos los comandos compose se registran:
```
log.Printf("Updating compose project %s with file: %s", project, composeFile)
log.Printf("Successfully updated compose project: %s", project)
```

## 🐛 Troubleshooting

### Error: "No se encontró el directorio del proyecto"
**Causa:** Label `com.docker.compose.project.working_dir` no existe o es inválido
**Solución:** Verificar que el contenedor fue desplegado con compose v2

### Error: "No se encontró archivo compose"
**Causa:** Archivo no existe o nombre diferente
**Solución:** Verificar que existe compose.yaml/yml o docker-compose.yaml/yml

### Error: "comando excedió timeout"
**Causa:** Pull o up tardó más del timeout
**Solución:** Aumentar timeout en código o verificar conectividad

### Error: "docker compose no está disponible"
**Causa:** docker-cli-compose no instalado en contenedor
**Solución:** Verificar Dockerfile incluye `apk add docker-cli-compose`

## ✅ Checklist de Validación

Antes de usar en producción:
- [x] Código compila sin errores
- [x] Docker Compose detectado al inicio
- [x] Archivos compose encontrados correctamente
- [x] Mapeo de directorios funciona
- [x] Timeouts configurados
- [x] Manejo de errores robusto
- [x] Logs detallados
- [ ] Probado con actualización real (pendiente prueba manual)

## 📝 Próximos Pasos

1. **Probar actualización real:** Ejecutar `/updateall` y actualizar un proyecto
2. **Verificar rollback:** Si falla, verificar que no deja contenedores en mal estado
3. **Probar con múltiples proyectos:** Verificar que funciona con diferentes proyectos compose
