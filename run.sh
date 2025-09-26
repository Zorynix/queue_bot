#!/bin/bash

if [ -f ".env" ]; then
    echo "📄 Загружаем переменные из .env файла..."
    export $(grep -v '^#' .env | xargs)
fi

if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo "❌ Ошибка: Переменная TELEGRAM_BOT_TOKEN не установлена"
    echo "Установите её командой: export TELEGRAM_BOT_TOKEN=\"your_token_here\""
    exit 1
fi

if [ -z "$GOOGLE_SHEETS_ID" ]; then
    echo "❌ Ошибка: Переменная GOOGLE_SHEETS_ID не установлена"
    echo "Установите её командой: export GOOGLE_SHEETS_ID=\"your_sheets_id_here\""
    exit 1
fi

if [ -z "$GOOGLE_CREDENTIALS_FILE" ] && [ -z "$GOOGLE_CREDENTIALS_JSON" ]; then
    echo "❌ Ошибка: Необходимо установить GOOGLE_CREDENTIALS_FILE или GOOGLE_CREDENTIALS_JSON"
    echo "Пример: export GOOGLE_CREDENTIALS_FILE=\"/path/to/credentials.json\""
    exit 1
fi

if [ ! -f "queue_lessons.txt" ]; then
    echo "❌ Ошибка: Файл queue_lessons.txt не найден"
    exit 1
fi

echo "🚀 Запуск Queue Bot..."
echo "📋 Загружено предметов: $(wc -l < queue_lessons.txt)"

if [ -f "user_mapping.json" ]; then
    echo "👥 Найден файл маппинга пользователей"
else
    echo "⚠️  Файл user_mapping.json не найден, будут использоваться имена из профилей Telegram"
fi

echo "🔄 Запуск..."
GOEXPERIMENT=greenteagc go run main.go