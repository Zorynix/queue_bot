package main

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	TelegramBotToken      string
	QueueChatID           int64
	GoogleSheetsID        string
	GoogleCredentialsFile string
	GoogleCredentialsJSON string
}

func LoadConfig() (*Config, error) {
	config := &Config{}

	config.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	if config.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	chatIDStr := os.Getenv("QUEUE_CHAT_ID")
	if chatIDStr == "" {
		return nil, fmt.Errorf("QUEUE_CHAT_ID environment variable is not set")
	}

	var err error
	config.QueueChatID, err = strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid QUEUE_CHAT_ID: %w", err)
	}

	config.GoogleSheetsID = os.Getenv("GOOGLE_SHEETS_ID")
	if config.GoogleSheetsID == "" {
		return nil, fmt.Errorf("GOOGLE_SHEETS_ID environment variable is not set")
	}

	config.GoogleCredentialsFile = os.Getenv("GOOGLE_CREDENTIALS_FILE")
	config.GoogleCredentialsJSON = os.Getenv("GOOGLE_CREDENTIALS_JSON")

	if config.GoogleCredentialsFile == "" && config.GoogleCredentialsJSON == "" {
		return nil, fmt.Errorf("either GOOGLE_CREDENTIALS_FILE or GOOGLE_CREDENTIALS_JSON must be set")
	}

	return config, nil
}
