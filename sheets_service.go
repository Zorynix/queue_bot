package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type SheetsService struct {
	service       *sheets.Service
	spreadsheetID string
	queueManager  *QueueManager
}

func NewSheetsService(config *Config, queueManager *QueueManager) (*SheetsService, error) {
	ctx := context.Background()

	var creds []byte
	var err error

	if config.GoogleCredentialsFile != "" {
		creds, err = os.ReadFile(config.GoogleCredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("error reading credentials file: %w", err)
		}
	} else {
		creds = []byte(config.GoogleCredentialsJSON)
	}

	jwtConfig, err := google.JWTConfigFromJSON(creds, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("error creating JWT config: %w", err)
	}

	client := jwtConfig.Client(ctx)

	service, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("error creating Sheets service: %w", err)
	}

	log.Println("Google Sheets API initialized successfully")

	return &SheetsService{
		service:       service,
		spreadsheetID: config.GoogleSheetsID,
		queueManager:  queueManager,
	}, nil
}

func (ss *SheetsService) AddToSheet(subjectName, userName string) error {
	readRange := "A:ZZ"
	resp, err := ss.service.Spreadsheets.Values.Get(ss.spreadsheetID, readRange).Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve data from sheet: %w", err)
	}

	if len(resp.Values) == 0 {
		return fmt.Errorf("no data found in sheet")
	}

	columnName, exists := ss.queueManager.GetColumnMapping(subjectName)
	if !exists {
		return fmt.Errorf("subject not found in column mapping: %s", subjectName)
	}

	headerRow := resp.Values[0]
	subjectColumn := -1
	for i, header := range headerRow {
		if headerStr, ok := header.(string); ok && strings.Contains(headerStr, columnName) {
			subjectColumn = i
			break
		}
	}

	if subjectColumn == -1 {
		return fmt.Errorf("subject column not found: %s (looking for column: %s)", subjectName, columnName)
	}

	targetRow := -1
	for i := 1; i < len(resp.Values); i++ {
		if subjectColumn >= len(resp.Values[i]) || resp.Values[i][subjectColumn] == "" {
			targetRow = i + 1
			break
		}
	}

	if targetRow == -1 {
		targetRow = len(resp.Values) + 1
	}

	columnLetter := numberToColumnLetter(subjectColumn + 1)
	writeRange := fmt.Sprintf("%s%d", columnLetter, targetRow)

	values := [][]interface{}{
		{userName},
	}

	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err = ss.service.Spreadsheets.Values.Update(ss.spreadsheetID, writeRange, valueRange).
		ValueInputOption("RAW").Do()

	if err != nil {
		return fmt.Errorf("unable to write data to sheet: %w", err)
	}

	log.Printf("Successfully added %s to %s -> %s (column %s, row %d)",
		userName, subjectName, columnName, columnLetter, targetRow)
	return nil
}

func (ss *SheetsService) ClearColumn(subjectName string) error {
	columnName, exists := ss.queueManager.GetColumnMapping(subjectName)
	if !exists {
		return fmt.Errorf("subject not found in column mapping: %s", subjectName)
	}

	readRange := "A:ZZ"
	resp, err := ss.service.Spreadsheets.Values.Get(ss.spreadsheetID, readRange).Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve data from sheet: %w", err)
	}

	if len(resp.Values) == 0 {
		return fmt.Errorf("no data found in sheet")
	}

	headerRow := resp.Values[0]
	subjectColumn := -1
	for i, header := range headerRow {
		if headerStr, ok := header.(string); ok && strings.Contains(headerStr, columnName) {
			subjectColumn = i
			break
		}
	}

	if subjectColumn == -1 {
		return fmt.Errorf("subject column not found: %s", columnName)
	}

	columnLetter := numberToColumnLetter(subjectColumn + 1)
	clearRange := fmt.Sprintf("%s2:%s", columnLetter, columnLetter)

	_, err = ss.service.Spreadsheets.Values.Clear(ss.spreadsheetID, clearRange, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return fmt.Errorf("unable to clear column in sheet: %w", err)
	}

	return nil
}

func (ss *SheetsService) RemoveFromSheet(subjectName, userName string) error {
	log.Printf("🗑️  Попытка удалить из Google Sheets: пользователь=%s, предмет=%s", userName, subjectName)

	columnName, exists := ss.queueManager.GetColumnMapping(subjectName)
	if !exists {
		return fmt.Errorf("no column mapping for subject: %s", subjectName)
	}

	log.Printf("🔍 Ищем колонку: %s", columnName)

	resp, err := ss.service.Spreadsheets.Values.Get(ss.spreadsheetID, "A1:Z").Do()
	if err != nil {
		return fmt.Errorf("unable to retrieve data from sheet: %w", err)
	}

	if len(resp.Values) == 0 {
		return fmt.Errorf("no data found in sheet")
	}

	headers := resp.Values[0]
	subjectColumn := -1
	for i, header := range headers {
		if headerStr, ok := header.(string); ok && strings.Contains(headerStr, columnName) {
			subjectColumn = i
			log.Printf("📍 Найдена колонка %s на позиции %d", columnName, i)
			break
		}
	}

	if subjectColumn == -1 {
		return fmt.Errorf("subject column not found: %s", columnName)
	}

	targetRow := -1
	log.Printf("🔍 Ищем фамилию '%s' в колонке %d", userName, subjectColumn)

	for i := 1; i < len(resp.Values); i++ {
		if subjectColumn < len(resp.Values[i]) {
			cellValue := ""
			if resp.Values[i][subjectColumn] != nil {
				cellValue = fmt.Sprintf("%v", resp.Values[i][subjectColumn])
			}
			log.Printf("📋 Строка %d, значение: '%s'", i+1, cellValue)

			if cellValue == userName {
				targetRow = i + 1
				log.Printf("✅ Найден пользователь '%s' в строке %d", userName, targetRow)
				break
			}
		}
	}

	if targetRow == -1 {
		log.Printf("❌ Пользователь '%s' не найден в колонке для предмета '%s'", userName, subjectName)
		return fmt.Errorf("user %s not found in queue for %s", userName, subjectName)
	}

	columnLetter := numberToColumnLetter(subjectColumn + 1)
	clearRange := fmt.Sprintf("%s%d", columnLetter, targetRow)
	log.Printf("🗑️  Очищаем ячейку: %s", clearRange)

	_, err = ss.service.Spreadsheets.Values.Clear(ss.spreadsheetID, clearRange, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return fmt.Errorf("unable to clear cell in sheet: %w", err)
	}

	log.Printf("✅ Успешно удален пользователь '%s' из Google Sheets", userName)
	return nil
}

func (ss *SheetsService) GetQueueFromSheet(subjectName string) ([]string, error) {
	columnName, exists := ss.queueManager.GetColumnMapping(subjectName)
	if !exists {
		return nil, fmt.Errorf("no column mapping for subject: %s", subjectName)
	}

	resp, err := ss.service.Spreadsheets.Values.Get(ss.spreadsheetID, "A1:Z").Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve data from sheet: %w", err)
	}

	if len(resp.Values) == 0 {
		return nil, fmt.Errorf("no data found in sheet")
	}

	headers := resp.Values[0]
	subjectColumn := -1
	for i, header := range headers {
		if headerStr, ok := header.(string); ok && strings.Contains(headerStr, columnName) {
			subjectColumn = i
			break
		}
	}

	if subjectColumn == -1 {
		return nil, fmt.Errorf("subject column not found: %s", columnName)
	}

	var queue []string
	for i := 1; i < len(resp.Values); i++ {
		if subjectColumn < len(resp.Values[i]) && resp.Values[i][subjectColumn] != nil {
			cellValue := strings.TrimSpace(fmt.Sprintf("%v", resp.Values[i][subjectColumn]))
			if cellValue != "" {
				queue = append(queue, cellValue)
			}
		}
	}

	log.Printf("📋 Очередь из Google Sheets для '%s': %v", subjectName, queue)
	return queue, nil
}
