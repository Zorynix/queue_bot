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
}

func NewNotificationService(bot *tgbotapi.BotAPI, queueManager *QueueManager, sheetsService *SheetsService, config *Config) *NotificationService {
	ns := &NotificationService{
		bot:               bot,
		queueManager:      queueManager,
		sheetsService:     sheetsService,
		config:            config,
		sentNotifications: make(map[string]time.Time),
	}

	ns.checkOnStartup()

	return ns
}

func (ns *NotificationService) checkOnStartup() {
	now := time.Now()
	subjects := ns.queueManager.GetSubjects()

	log.Println("🔍 Проверяем предметы при запуске...")

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
		}
	}

	log.Println("✅ Проверка при запуске завершена")
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
	queueButton := tgbotapi.NewInlineKeyboardButtonData("Текущая очередь", fmt.Sprintf("queue_%s", shortCode))
	keyboard := tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{joinButton, queueButton})

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
	} else if strings.HasPrefix(data, "queue_") {
		shortCode := strings.TrimPrefix(data, "queue_")
		subjectName := ns.findSubjectByShortCode(shortCode)
		if subjectName != "" {
			ns.handleShowQueue(callbackQuery, subjectName)
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
		emptyCallback := tgbotapi.NewCallback(callbackQuery.ID, "")
		ns.bot.Request(emptyCallback)

		msg := tgbotapi.NewMessage(int64(callbackQuery.From.ID), "❌ Не удалось определить ваше реальное имя. Обратитесь к администратору.")
		ns.bot.Send(msg)
		return
	}

	position, joined := ns.queueManager.JoinQueue(subjectName, realName)
	if !joined {
		emptyCallback := tgbotapi.NewCallback(callbackQuery.ID, "")
		ns.bot.Request(emptyCallback)

		msg := tgbotapi.NewMessage(int64(callbackQuery.From.ID), "❌ Вы уже записаны в очередь на этот предмет!")
		ns.bot.Send(msg)
		return
	}

	lastName := extractLastName(realName)
	if err := ns.sheetsService.AddToSheet(subjectName, lastName); err != nil {
		log.Printf("Error adding to Google Sheets: %v", err)
		ns.queueManager.RemoveFromQueue(subjectName, realName)

		emptyCallback := tgbotapi.NewCallback(callbackQuery.ID, "")
		ns.bot.Request(emptyCallback)

		msg := tgbotapi.NewMessage(int64(callbackQuery.From.ID), "❌ Ошибка при записи в таблицу. Попробуйте позже.")
		ns.bot.Send(msg)
		return
	}

	emptyCallback := tgbotapi.NewCallback(callbackQuery.ID, "")
	ns.bot.Request(emptyCallback)

	queuePosition, previousUser, _ := ns.queueManager.GetQueueInfo(subjectName, realName)

	var messageText string
	if queuePosition == 1 {
		messageText = fmt.Sprintf("✅ Вы успешно записались на предмет \"%s\"!\n👤 Вы первый в очереди!", subjectName)
	} else if previousUser != "" {
		previousLastName := extractLastName(previousUser)
		messageText = fmt.Sprintf("✅ Вы успешно записались на предмет \"%s\"!\n👤 Ваше место в очереди: %d\n📝 Вы идете после: %s",
			subjectName, queuePosition, previousLastName)
	} else {
		messageText = fmt.Sprintf("✅ Вы успешно записались на предмет \"%s\"!\n👤 Ваше место в очереди: %d",
			subjectName, queuePosition)
	}

	msg := tgbotapi.NewMessage(int64(callbackQuery.From.ID), messageText)
	if _, err := ns.bot.Send(msg); err != nil {
		log.Printf("Error sending success message to user %s: %v", realName, err)
		groupMsg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, messageText)
		ns.bot.Send(groupMsg)
	}

	log.Printf("User %s joined queue for %s (position %d)", realName, subjectName, position)
}

func (ns *NotificationService) handleShowQueue(callbackQuery *tgbotapi.CallbackQuery, subjectName string) {
	emptyCallback := tgbotapi.NewCallback(callbackQuery.ID, "")
	ns.bot.Request(emptyCallback)

	queue := ns.queueManager.GetQueue(subjectName)

	var messageText string
	if len(queue) == 0 {
		messageText = fmt.Sprintf("📋 Текущая очередь по предмету \"%s\":\n\n❌ Очередь пуста", subjectName)
	} else {
		messageText = fmt.Sprintf("📋 Текущая очередь по предмету \"%s\":\n\n", subjectName)
		for i, person := range queue {
			lastName := extractLastName(person)
			messageText += fmt.Sprintf("%d. %s\n", i+1, lastName)
		}
	}

	msg := tgbotapi.NewMessage(int64(callbackQuery.From.ID), messageText)
	if _, err := ns.bot.Send(msg); err != nil {
		log.Printf("Error sending queue info to user: %v", err)
		groupMsg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, messageText)
		ns.bot.Send(groupMsg)
	}
}

func extractLastName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) > 0 {
		return parts[0]
	}
	return fullName
}
