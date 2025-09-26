package main

import (
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)
	log.Println("Отправьте боту любое сообщение, чтобы получить Chat ID...")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			chatID := update.Message.Chat.ID
			chatType := update.Message.Chat.Type
			userName := update.Message.From.UserName
			firstName := update.Message.From.FirstName

			log.Printf("=== CHAT INFO ===")
			log.Printf("Chat ID: %d", chatID)
			log.Printf("Chat Type: %s", chatType)
			log.Printf("User: %s (@%s)", firstName, userName)
			log.Printf("=================")

			msg := tgbotapi.NewMessage(chatID,
				fmt.Sprintf("🆔 Ваш Chat ID: `%d`\n📱 Тип чата: %s\n👤 Пользователь: %s (@%s)",
					chatID, chatType, firstName, userName))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
		}
	}
}
