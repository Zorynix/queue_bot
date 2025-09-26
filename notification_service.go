package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type NotificationService struct {
	bot               *tgbotapi.BotAPI
	queueManager      *QueueManager
	sheetsService     *SheetsService
	config            *Config
	sentNotifications map[string]time.Time
	queueMessageIDs   map[string]int
}

func NewNotificationService(bot *tgbotapi.BotAPI, queueManager *QueueManager, sheetsService *SheetsService, config *Config) *NotificationService {
	ns := &NotificationService{
		bot:               bot,
		queueManager:      queueManager,
		sheetsService:     sheetsService,
		config:            config,
		sentNotifications: make(map[string]time.Time),
		queueMessageIDs:   make(map[string]int),
	}

	log.Println("🔄 Синхронизация очередей с Google Sheets при запуске...")
	ns.syncAllQueuesFromSheets()

	ns.checkOnStartup()

	return ns
}

func (ns *NotificationService) checkOnStartup() {
	now := time.Now()
	subjects := ns.queueManager.GetSubjects()

	log.Println("🔍 Проверяем предметы при запуске...")

	subjectsFound := 0
	for _, subject := range subjects {
		nextSubjectTime := GetNextSubjectTime(subject)
		if nextSubjectTime == nil {
			continue
		}

		timeUntilSubject := nextSubjectTime.Sub(now)

		if timeUntilSubject > 0 && timeUntilSubject <= 24*time.Hour {
			log.Printf("📚 Найден предмет в ближайшие 24 часа: %s (через %v)",
				subject.Name, timeUntilSubject.Round(time.Minute))

			ns.sendQueueNotification(subject)
			subjectsFound++
		}
	}

	if subjectsFound > 0 {
		log.Printf("✅ Проверка при запуске завершена. Отправлено уведомлений: %d", subjectsFound)
	} else {
		log.Println("✅ Проверка при запуске завершена. Предметов в ближайшие 24 часа не найдено")
	}
}

func (ns *NotificationService) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Notification scheduler stopped")
			return
		case <-ticker.C:
			ns.checkAndSendNotifications()
			ns.checkAndClearFinishedSubjects()
		case <-cleanupTicker.C:
			ns.cleanupOldNotifications()
		}
	}
}

func (ns *NotificationService) checkAndSendNotifications() {
	now := time.Now()
	subjects := ns.queueManager.GetSubjects()

	for _, subject := range subjects {
		nextSubjectTime := GetNextSubjectTime(subject)
		if nextSubjectTime == nil {
			continue
		}

		timeUntilSubject := nextSubjectTime.Sub(now)
		if timeUntilSubject > 23*time.Hour+50*time.Minute && timeUntilSubject < 24*time.Hour+10*time.Minute {
			ns.sendQueueNotification(subject)
		}
	}
}

func (ns *NotificationService) checkAndClearFinishedSubjects() {
	now := time.Now()
	subjects := ns.queueManager.GetSubjects()

	for _, subject := range subjects {
		endTime := GetNextSubjectEndTime(subject)
		if endTime != nil && now.After(*endTime) {
			ns.clearSubjectQueue(subject.Name)
		}
	}
}

func (ns *NotificationService) sendQueueNotification(subject Subject) {
	now := time.Now()

	nextSubjectTime := GetNextSubjectTime(subject)
	if nextSubjectTime == nil {
		return
	}

	notificationKey := fmt.Sprintf("%s_%s", subject.Name, nextSubjectTime.Format("2006-01-02"))

	if lastSent, exists := ns.sentNotifications[notificationKey]; exists {
		if now.Sub(lastSent) < 6*time.Hour {
			log.Printf("⏭️  Пропускаем уведомление для %s - уже отправлено %v назад",
				subject.Name, now.Sub(lastSent).Round(time.Minute))
			return
		}
	}

	text := "📚 Открыта запись в очередь на сдачу работ!\n\n"
	text += fmt.Sprintf("🎓 **%s**\n", subject.Name)
	text += fmt.Sprintf("📅 %s в %s-%s\n\n", subject.Day, subject.Start, subject.End)
	text += "Нажмите кнопку ниже, чтобы записаться в очередь:"

	shortCode, exists := ns.queueManager.GetColumnMapping(subject.Name)
	if !exists {
		log.Printf("Warning: No short code found for subject: %s", subject.Name)
		return
	}

	joinButton := tgbotapi.NewInlineKeyboardButtonData("Записаться", fmt.Sprintf("join_%s", shortCode))
	leaveButton := tgbotapi.NewInlineKeyboardButtonData("Уйти из очереди", fmt.Sprintf("leave_%s", shortCode))
	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{joinButton, leaveButton})

	msg := tgbotapi.NewMessage(ns.config.QueueChatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard

	if _, err := ns.bot.Send(msg); err != nil {
		log.Printf("Error sending queue notification for %s: %v", subject.Name, err)
		return
	}

	ns.sentNotifications[notificationKey] = now

	log.Printf("✅ Sent queue notification for subject: %s", subject.Name)
}

func (ns *NotificationService) clearSubjectQueue(subjectName string) {
	ns.queueManager.ClearQueue(subjectName)

	if err := ns.sheetsService.ClearColumn(subjectName); err != nil {
		log.Printf("Error clearing Google Sheets column for %s: %v", subjectName, err)
	} else {
		log.Printf("Cleared queue and Google Sheets column for subject: %s", subjectName)
	}
}

func (ns *NotificationService) cleanupOldNotifications() {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -1)

	for key, sentTime := range ns.sentNotifications {
		if sentTime.Before(cutoff) {
			delete(ns.sentNotifications, key)
		}
	}
}

func (ns *NotificationService) HandleCallbackQuery(callbackQuery *tgbotapi.CallbackQuery) {
	data := callbackQuery.Data

	if strings.HasPrefix(data, "join_") {
		shortCode := strings.TrimPrefix(data, "join_")
		subjectName := ns.findSubjectByShortCode(shortCode)
		if subjectName != "" {
			ns.handleJoinQueue(callbackQuery, subjectName)
		} else {
			callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Предмет не найден")
			ns.bot.Request(callback)
		}
	} else if strings.HasPrefix(data, "leave_") {
		shortCode := strings.TrimPrefix(data, "leave_")
		subjectName := ns.findSubjectByShortCode(shortCode)
		if subjectName != "" {
			ns.handleLeaveQueue(callbackQuery, subjectName)
		} else {
			callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Предмет не найден")
			ns.bot.Request(callback)
		}
	}
}

func (ns *NotificationService) findSubjectByShortCode(shortCode string) string {
	subjects := ns.queueManager.GetSubjects()
	for _, subject := range subjects {
		if mappedCode, exists := ns.queueManager.GetColumnMapping(subject.Name); exists && mappedCode == shortCode {
			return subject.Name
		}
	}
	return ""
}

func (ns *NotificationService) handleJoinQueue(callbackQuery *tgbotapi.CallbackQuery, subjectName string) {
	user := callbackQuery.From

	realName := ns.queueManager.GetUserRealName(user.UserName, user.FirstName, user.LastName)
	if realName == "" {
		callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Не удалось определить ваше реальное имя")
		ns.bot.Request(callback)
		return
	}

	if err := ns.syncQueueFromSheets(subjectName); err != nil {
		log.Printf("Warning: Could not sync with Google Sheets: %v", err)
	}

	lastName := extractLastName(realName)

	currentPosition := ns.queueManager.GetUserPositionInQueue(subjectName, realName)
	if currentPosition > 0 {
		callback := tgbotapi.NewCallback(callbackQuery.ID, fmt.Sprintf("✅ Вы уже в очереди! Место: %d", currentPosition))
		ns.bot.Request(callback)
		return
	}

	if err := ns.sheetsService.AddToSheet(subjectName, lastName); err != nil {
		log.Printf("Error adding to Google Sheets: %v", err)
		callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Ошибка при записи в таблицу")
		ns.bot.Request(callback)
		return
	}

	if err := ns.syncQueueFromSheets(subjectName); err != nil {
		log.Printf("Error syncing after adding to sheets: %v", err)
	}

	position := ns.queueManager.GetUserPositionInQueue(subjectName, realName)
	if position == -1 {
		position = 1
	}

	if err := ns.sheetsService.AddToSheet(subjectName, lastName); err != nil {
		log.Printf("Error adding to Google Sheets: %v", err)
		ns.queueManager.RemoveFromQueue(subjectName, realName)

		callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Ошибка при записи в таблицу")
		ns.bot.Request(callback)
		return
	}

	callback := tgbotapi.NewCallback(callbackQuery.ID, "✅ Вы записались в очередь!")
	ns.bot.Request(callback)

	lastName = extractLastName(realName)
	chatMessage := fmt.Sprintf("✅ %s записался в очередь на \"%s\" (место: %d)", lastName, subjectName, position)

	msg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, chatMessage)
	if _, err := ns.bot.Send(msg); err != nil {
		log.Printf("Error sending chat message: %v", err)
	}

	ns.updateOrCreateQueueMessage(callbackQuery.Message.Chat.ID, subjectName)

	log.Printf("User %s joined queue for %s (position %d)", realName, subjectName, position)
}

func (ns *NotificationService) updateOrCreateQueueMessage(chatID int64, subjectName string) {
	queue := ns.queueManager.GetQueue(subjectName)
	var queueMessage string
	if len(queue) == 0 {
		queueMessage = fmt.Sprintf("📋 Текущая очередь на \"%s\":\n\n❌ Очередь пуста", subjectName)
	} else {
		queueMessage = fmt.Sprintf("📋 Текущая очередь на \"%s\":\n\n", subjectName)
		for i, person := range queue {
			personLastName := extractLastName(person)
			queueMessage += fmt.Sprintf("%d. %s\n", i+1, personLastName)
		}
	}

	if messageID, exists := ns.queueMessageIDs[subjectName]; exists {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, queueMessage)
		if _, err := ns.bot.Send(editMsg); err != nil {
			log.Printf("Error updating queue message: %v", err)

			ns.createNewQueueMessage(chatID, subjectName, queueMessage)
		}
	} else {
		ns.createNewQueueMessage(chatID, subjectName, queueMessage)
	}
}

func (ns *NotificationService) createNewQueueMessage(chatID int64, subjectName string, queueMessage string) {
	queueMsg := tgbotapi.NewMessage(chatID, queueMessage)
	sentMsg, err := ns.bot.Send(queueMsg)
	if err != nil {
		log.Printf("Error sending queue message: %v", err)
		return
	}

	ns.queueMessageIDs[subjectName] = sentMsg.MessageID
}

func (ns *NotificationService) handleLeaveQueue(callbackQuery *tgbotapi.CallbackQuery, subjectName string) {
	user := callbackQuery.From

	realName := ns.queueManager.GetUserRealName(user.UserName, user.FirstName, user.LastName)
	if realName == "" {
		callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Не удалось определить ваше реальное имя")
		ns.bot.Request(callback)
		return
	}

	queue := ns.queueManager.GetQueue(subjectName)
	found := false
	for _, person := range queue {
		if person == realName {
			found = true
			break
		}
	}

	if !found {
		callback := tgbotapi.NewCallback(callbackQuery.ID, "❌ Вы не записаны в очередь на этот предмет!")
		ns.bot.Request(callback)
		return
	}

	ns.queueManager.RemoveFromQueue(subjectName, realName)

	lastName := extractLastName(realName)
	if err := ns.sheetsService.RemoveFromSheet(subjectName, lastName); err != nil {
		log.Printf("Error removing from Google Sheets: %v", err)
	}

	callback := tgbotapi.NewCallback(callbackQuery.ID, "✅ Вы вышли из очереди!")
	ns.bot.Request(callback)

	chatMessage := fmt.Sprintf("❌ %s вышел из очереди на \"%s\"", lastName, subjectName)

	msg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, chatMessage)
	if _, err := ns.bot.Send(msg); err != nil {
		log.Printf("Error sending leave message: %v", err)
	}

	ns.updateOrCreateQueueMessage(callbackQuery.Message.Chat.ID, subjectName)

	log.Printf("User %s left queue for %s", realName, subjectName)
}

func (ns *NotificationService) syncQueueFromSheets(subjectName string) error {
	queueFromSheets, err := ns.sheetsService.GetQueueFromSheet(subjectName)
	if err != nil {
		return fmt.Errorf("failed to get queue from sheets: %w", err)
	}

	var fullNamesQueue []string
	for _, lastName := range queueFromSheets {
		fullName := ns.findFullNameByLastName(lastName)
		if fullName != "" {
			fullNamesQueue = append(fullNamesQueue, fullName)
		} else {
			fullNamesQueue = append(fullNamesQueue, lastName)
		}
	}

	ns.queueManager.SyncWithSheets(subjectName, fullNamesQueue)
	return nil
}

func (ns *NotificationService) findFullNameByLastName(lastName string) string {
	userMappings := ns.queueManager.GetUserMappings()

	for _, realName := range userMappings {
		if extractLastName(realName) == lastName {
			return realName
		}
	}

	return ""
}

func (ns *NotificationService) syncAllQueuesFromSheets() {
	subjects := ns.queueManager.GetSubjects()

	for _, subject := range subjects {
		log.Printf("🔄 Синхронизация очереди для предмета: %s", subject.Name)

		sheetsQueue, err := ns.sheetsService.GetQueueFromSheet(subject.Name)
		if err != nil {
			log.Printf("⚠️  Ошибка при получении очереди из Google Sheets для %s: %v", subject.Name, err)
			continue
		}

		var fullNameQueue []string
		for _, lastName := range sheetsQueue {
			fullName := ns.findFullNameByLastName(lastName)
			if fullName != "" {
				fullNameQueue = append(fullNameQueue, fullName)
			} else {
				log.Printf("⚠️  Не найдено полное имя для фамилии: %s", lastName)
			}
		}

		ns.queueManager.SyncQueueFromSheets(subject.Name, fullNameQueue)

		log.Printf("✅ Синхронизировано %d пользователей для предмета %s", len(fullNameQueue), subject.Name)
	}

	log.Println("✅ Синхронизация всех очередей завершена")
}

func extractLastName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) > 0 {
		return parts[0]
	}
	return fullName
}
