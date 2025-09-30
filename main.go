package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	if err := LoadEnv(".env"); err != nil {
		log.Println("Warning: Could not load .env file:", err)
	}

	config, err := LoadConfig()
	if err != nil {
		log.Fatal("Error loading config:", err)
	}

	queueManager := NewQueueManager()

	if err := queueManager.LoadSubjects("queue_lessons.txt"); err != nil {
		log.Fatal("Error loading subjects:", err)
	}

	if err := queueManager.LoadUserMapping("user_mapping.json"); err != nil {
		log.Fatal("Error loading user mapping:", err)
	}

	sheetsService, err := NewSheetsService(config, queueManager)
	if err != nil {
		log.Fatal("Error initializing Google Sheets service:", err)
	}

	if err := sheetsService.RestoreColumnHeaders(); err != nil {
		log.Printf("Warning: Could not restore column headers: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(config.TelegramBotToken)
	if err != nil {
		log.Fatal("Error creating bot:", err)
	}

	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)

	notificationService := NewNotificationService(bot, queueManager, sheetsService, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go notificationService.StartScheduler(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := bot.GetUpdatesChan(u)

		for update := range updates {
			if update.CallbackQuery != nil {
				go notificationService.HandleCallbackQuery(update.CallbackQuery)
			}
		}
	}()

	<-sigChan
	log.Println("Shutting down bot gracefully...")
	cancel()
}
