package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type QueueManager struct {
	mu            sync.RWMutex
	subjectQueues map[string][]string
	userMapping   map[string]string
	subjects      []Subject
	columnMapping map[string]string
}

func NewQueueManager() *QueueManager {
	return &QueueManager{
		subjectQueues: make(map[string][]string),
		userMapping:   make(map[string]string),
		subjects:      make([]Subject, 0),
		columnMapping: map[string]string{
			"Микросервисная архитектура":                             "Микросервисы",
			"Стандартизация и сертификация программного обеспечения": "СИСПО",
			"Сопровождение программных систем":                       "СПС",
			"Управление информационно-технологическими проектами":    "УИТП",
			"Оценка параметров функционирования программных систем":  "ОПФПС",
			"Проектирование программных систем":                      "ППС",
			"Технологии и инструментарий анализа больших данных":     "БИГДАТА",
		},
	}
}

func (qm *QueueManager) LoadSubjects(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", filename, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("error reading CSV: %w", err)
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()

	qm.subjects = make([]Subject, 0, len(records))
	for _, record := range records {
		if len(record) >= 4 {
			qm.subjects = append(qm.subjects, Subject{
				Day:   record[0],
				Start: record[1],
				Name:  record[2],
				End:   record[3],
			})
		}
	}

	log.Printf("Loaded %d subjects", len(qm.subjects))
	return nil
}

func (qm *QueueManager) LoadUserMapping(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Println("Warning: user_mapping.json not found, creating empty mapping")
		return nil
	}
	defer file.Close()

	var mappings []UserMapping
	if err := json.NewDecoder(file).Decode(&mappings); err != nil {
		return fmt.Errorf("error decoding user mapping: %w", err)
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()

	qm.userMapping = make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		qm.userMapping[mapping.TelegramUsername] = mapping.RealName
	}

	log.Printf("Loaded %d user mappings", len(qm.userMapping))
	return nil
}

func (qm *QueueManager) GetSubjects() []Subject {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	subjects := make([]Subject, len(qm.subjects))
	copy(subjects, qm.subjects)
	return subjects
}

func (qm *QueueManager) GetUserRealName(username, firstName, lastName string) string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	if username != "" {
		if realName, exists := qm.userMapping[username]; exists {
			return realName
		}
	}

	return ""
}

func (qm *QueueManager) JoinQueue(subjectName, realName string) (int, bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	queue := qm.subjectQueues[subjectName]

	for i, name := range queue {
		if name == realName {
			log.Printf("⚠️  Пользователь %s уже есть в очереди %s на позиции %d", realName, subjectName, i+1)
			return i + 1, false
		}
	}

	qm.subjectQueues[subjectName] = append(queue, realName)
	log.Printf("✅ Пользователь %s добавлен в очередь %s на позицию %d", realName, subjectName, len(qm.subjectQueues[subjectName]))
	return len(qm.subjectQueues[subjectName]), true
}

func (qm *QueueManager) RemoveFromQueue(subjectName, realName string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	queue := qm.subjectQueues[subjectName]
	for i, name := range queue {
		if name == realName {
			qm.subjectQueues[subjectName] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
}

func (qm *QueueManager) ClearQueue(subjectName string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	delete(qm.subjectQueues, subjectName)
}

func (qm *QueueManager) GetColumnMapping(subjectName string) (string, bool) {
	columnName, exists := qm.columnMapping[subjectName]
	return columnName, exists
}

func (qm *QueueManager) GetQueueInfo(subjectName, realName string) (position int, previousUser string, found bool) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	queue, exists := qm.subjectQueues[subjectName]
	if !exists {
		return 0, "", false
	}

	for i, name := range queue {
		if name == realName {
			position = i + 1
			if i > 0 {
				previousUser = queue[i-1]
			}
			return position, previousUser, true
		}
	}

	return 0, "", false
}

func GetNextSubjectTime(subject Subject) *time.Time {
	now := time.Now()

	startTime, err := time.Parse("15:04", subject.Start)
	if err != nil {
		log.Printf("Error parsing start time for %s: %v", subject.Name, err)
		return nil
	}

	targetWeekday := parseWeekday(subject.Day)
	if targetWeekday == -1 {
		log.Printf("Error parsing weekday for %s: %s", subject.Name, subject.Day)
		return nil
	}

	daysUntil := (int(targetWeekday) - int(now.Weekday()) + 7) % 7
	if daysUntil == 0 {
		currentTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
		subjectTime := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, now.Location())

		if currentTime.After(subjectTime) {
			daysUntil = 7
		}
	}

	nextDate := now.AddDate(0, 0, daysUntil)
	nextSubjectTime := time.Date(nextDate.Year(), nextDate.Month(), nextDate.Day(),
		startTime.Hour(), startTime.Minute(), 0, 0, now.Location())

	return &nextSubjectTime
}

func GetNextSubjectEndTime(subject Subject) *time.Time {
	startTime := GetNextSubjectTime(subject)
	if startTime == nil {
		return nil
	}

	endTime, err := time.Parse("15:04", subject.End)
	if err != nil {
		log.Printf("Error parsing end time for %s: %v", subject.Name, err)
		return nil
	}

	nextEndTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(),
		endTime.Hour(), endTime.Minute(), 0, 0, startTime.Location())

	return &nextEndTime
}

func (qm *QueueManager) GetQueue(subjectName string) []string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	queue, exists := qm.subjectQueues[subjectName]
	if !exists {
		return []string{}
	}

	result := make([]string, len(queue))
	copy(result, queue)
	return result
}

func (qm *QueueManager) SyncWithSheets(subjectName string, queueFromSheets []string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	var cleanQueue []string
	seen := make(map[string]bool)
	duplicatesCount := 0

	for _, name := range queueFromSheets {
		cleanName := strings.TrimSpace(name)
		if cleanName != "" {
			if seen[cleanName] {
				duplicatesCount++
				log.Printf("⚠️  Обнаружен дубликат в Google Sheets: %s для предмета %s", cleanName, subjectName)
			} else {
				cleanQueue = append(cleanQueue, cleanName)
				seen[cleanName] = true
			}
		}
	}

	if duplicatesCount > 0 {
		log.Printf("🔄 Синхронизация очереди для предмета '%s': %v (удалено дубликатов: %d)", subjectName, cleanQueue, duplicatesCount)
	} else {
		log.Printf("🔄 Синхронизация очереди для предмета '%s': %v", subjectName, cleanQueue)
	}
	qm.subjectQueues[subjectName] = cleanQueue
}

func (qm *QueueManager) GetUserPositionInQueue(subjectName, realName string) int {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	queue, exists := qm.subjectQueues[subjectName]
	if !exists {
		return -1
	}

	for i, name := range queue {
		if name == realName {
			return i + 1
		}
	}

	return -1
}

func (qm *QueueManager) GetUserMappings() map[string]string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make(map[string]string, len(qm.userMapping))
	for k, v := range qm.userMapping {
		result[k] = v
	}
	return result
}

func (qm *QueueManager) SyncQueueFromSheets(subjectName string, queue []string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	qm.subjectQueues[subjectName] = make([]string, len(queue))
	copy(qm.subjectQueues[subjectName], queue)

	log.Printf("🔄 Очередь для %s синхронизирована: %d пользователей", subjectName, len(queue))
}
