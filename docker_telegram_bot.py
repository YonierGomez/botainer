#!/usr/bin/env python3
import os
import sys
import subprocess
import logging
import asyncio
from dotenv import load_dotenv
from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.ext import Application, CommandHandler, CallbackQueryHandler, ContextTypes

# Fix for Python 3.14+ event loop
if sys.version_info >= (3, 10):
    try:
        asyncio.get_event_loop()
    except RuntimeError:
        asyncio.set_event_loop(asyncio.new_event_loop())

load_dotenv()

logging.basicConfig(format='%(asctime)s - %(name)s - %(levelname)s - %(message)s', level=logging.INFO)
logger = logging.getLogger(__name__)

TOKEN = os.getenv('TELEGRAM_BOT_TOKEN')
COMPOSE_PROJECT = os.getenv('COMPOSE_PROJECT', 'work_pro')
COMPOSE_FILE = os.getenv('COMPOSE_FILE', 'docker-compose.yml')

def detect_compose_file():
    """Auto-detect compose file if not specified"""
    possible_files = ['docker-compose.yml', 'docker-compose.yaml', 'compose.yml', 'compose.yaml']
    for f in possible_files:
        if os.path.exists(f):
            return f
    return COMPOSE_FILE

COMPOSE_FILE = detect_compose_file() if COMPOSE_FILE == 'docker-compose.yml' else COMPOSE_FILE

def run_cmd(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=300)
        return f"✅ Exitoso:\n```\n{result.stdout}\n```" if result.returncode == 0 else f"❌ Error:\n```\n{result.stderr}\n```"
    except Exception as e:
        return f"❌ Excepción: {str(e)}"

async def start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    keyboard = [
        [InlineKeyboardButton("🔄 Pull & Up", callback_data='pull_up')],
        [InlineKeyboardButton("📋 Estado", callback_data='status'), InlineKeyboardButton("📝 Listar", callback_data='list')],
        [InlineKeyboardButton("📊 Logs", callback_data='logs'), InlineKeyboardButton("🔄 Restart", callback_data='restart_all')],
        [InlineKeyboardButton("⏸️ Detener", callback_data='stop'), InlineKeyboardButton("▶️ Iniciar", callback_data='start_all')],
        [InlineKeyboardButton("🗑️ Eliminar", callback_data='down'), InlineKeyboardButton("🔍 Imágenes", callback_data='images')],
        [InlineKeyboardButton("💾 Volúmenes", callback_data='volumes'), InlineKeyboardButton("🌐 Redes", callback_data='networks')],
    ]
    await update.message.reply_text('🐳 Docker Compose Manager', reply_markup=InlineKeyboardMarkup(keyboard))

async def button(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    await query.answer()
    
    cmd_map = {
        'pull_up': f'docker compose -p {COMPOSE_PROJECT} pull && docker compose -p {COMPOSE_PROJECT} up -d',
        'status': f'docker compose -p {COMPOSE_PROJECT} ps',
        'list': f'docker compose -p {COMPOSE_PROJECT} config --services',
        'logs': f'docker compose -p {COMPOSE_PROJECT} logs --tail=50',
        'stop': f'docker compose -p {COMPOSE_PROJECT} stop',
        'start_all': f'docker compose -p {COMPOSE_PROJECT} start',
        'restart_all': f'docker compose -p {COMPOSE_PROJECT} restart',
        'down': f'docker compose -p {COMPOSE_PROJECT} down',
        'images': f'docker compose -p {COMPOSE_PROJECT} images',
        'volumes': 'docker volume ls',
        'networks': 'docker network ls',
    }
    
    if query.data in cmd_map:
        await query.edit_message_text(f"⏳ Ejecutando...")
        result = run_cmd(cmd_map[query.data])
        await query.edit_message_text(result, parse_mode='Markdown')

async def pull(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text("⏳ Descargando imágenes...")
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} pull')
    await update.message.reply_text(result, parse_mode='Markdown')

async def up(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text("⏳ Iniciando servicios...")
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} up -d')
    await update.message.reply_text(result, parse_mode='Markdown')

async def restart(update: Update, context: ContextTypes.DEFAULT_TYPE):
    service = ' '.join(context.args) if context.args else ''
    await update.message.reply_text(f"⏳ Reiniciando {service or 'todos'}...")
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} restart {service}')
    await update.message.reply_text(result, parse_mode='Markdown')

async def logs(update: Update, context: ContextTypes.DEFAULT_TYPE):
    service = ' '.join(context.args) if context.args else ''
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} logs --tail=100 {service}')
    await update.message.reply_text(result, parse_mode='Markdown')

async def list_services(update: Update, context: ContextTypes.DEFAULT_TYPE):
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} config --services')
    await update.message.reply_text(result, parse_mode='Markdown')

async def ps(update: Update, context: ContextTypes.DEFAULT_TYPE):
    result = run_cmd(f'docker compose -p {COMPOSE_PROJECT} ps -a')
    await update.message.reply_text(result, parse_mode='Markdown')

async def running(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Show all running containers on the server"""
    result = run_cmd('docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"')
    await update.message.reply_text(f"🐳 Contenedores corriendo:\n{result}", parse_mode='Markdown')

async def exec_cmd(update: Update, context: ContextTypes.DEFAULT_TYPE):
    if not context.args:
        await update.message.reply_text("Uso: /exec <comando>")
        return
    cmd = ' '.join(context.args)
    result = run_cmd(cmd)
    await update.message.reply_text(result, parse_mode='Markdown')

def main():
    if not TOKEN:
        logger.error("TELEGRAM_BOT_TOKEN no configurado")
        return
    
    app = Application.builder().token(TOKEN).build()
    app.add_handler(CommandHandler("start", start))
    app.add_handler(CommandHandler("pull", pull))
    app.add_handler(CommandHandler("up", up))
    app.add_handler(CommandHandler("restart", restart))
    app.add_handler(CommandHandler("logs", logs))
    app.add_handler(CommandHandler("list", list_services))
    app.add_handler(CommandHandler("ps", ps))
    app.add_handler(CommandHandler("running", running))
    app.add_handler(CommandHandler("exec", exec_cmd))
    app.add_handler(CallbackQueryHandler(button))
    
    logger.info("Bot iniciado")
    app.run_polling()

if __name__ == '__main__':
    main()
