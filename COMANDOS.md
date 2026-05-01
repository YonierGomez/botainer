# Comandos del Bot Botainer

## Comandos de texto
- /start - Menú principal con botones
- /ps - Contenedores corriendo con CPU/RAM y proyecto
- /running - Todos los contenedores (corriendo y detenidos)
- /restart - Grid para seleccionar contenedor a reiniciar
- /stop - Grid para seleccionar contenedor a detener
- /logs - Grid para seleccionar contenedor para ver logs
- /start_container - Grid para iniciar contenedores detenidos
- /images - Listar imágenes con botones (Inspect/Delete)
- /volumes - Listar volúmenes con uso y proyecto
- /networks - Listar redes con contenedores y proyecto
- /updateall - Actualizar todas las imágenes y recrear contenedores
- /notify - Activar notificaciones

## Botones del menú /start
- 📋 Estado (ps) - Ver contenedores corriendo
- 🔄 Restart - Reiniciar contenedor
- 📊 Logs - Ver logs
- ⏸️ Stop - Detener contenedor
- 🖼️ Images - Listar imágenes
- 💾 Volumes - Listar volúmenes
- 🌐 Networks - Listar redes
- 🔄 Update All - Actualizar todas las imágenes
- 🔔 Notificaciones - Activar notificaciones

## Acciones de botones inline
### Contenedores:
- logs:nombre - Ver logs del contenedor
- restart:nombre - Reiniciar contenedor
- stop:nombre - Detener contenedor
- start:nombre - Iniciar contenedor
- remove:nombre - Eliminar contenedor
- inspect:nombre - Inspeccionar contenedor

### Imágenes:
- inspect_img:id - Inspeccionar imagen
- rmi:id - Eliminar imagen

### Volúmenes:
- inspect_vol:nombre - Inspeccionar volumen
- rmvol:nombre - Eliminar volumen

### Redes:
- inspect_net:nombre - Inspeccionar red
- rmnet:nombre - Eliminar red

### Actualizaciones:
- update_compose:proyecto - Actualizar proyecto compose
- recreate:nombre - Recrear contenedor con nueva imagen

## Notificaciones automáticas
- ▶️ Contenedor iniciado
- ⏸️ Contenedor detenido
- 💀 Contenedor murió
- 🔄 Contenedor reiniciado
- 🗑️ Contenedor eliminado
- 🆕 Nueva versión de imagen disponible (con botón para actualizar)
