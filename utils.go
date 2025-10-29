package main

import (
	"log"
	"strings"
	"time"
)

func parseWeekday(day string) time.Weekday {
	switch strings.ToLower(day) {
	case "пн":
		return time.Monday
	case "вт":
		return time.Tuesday
	case "ср":
		return time.Wednesday
	case "чт":
		return time.Thursday
	case "пт":
		return time.Friday
	case "сб":
		return time.Saturday
	case "вс":
		return time.Sunday
	default:
		return -1
	}
}

func numberToColumnLetter(num int) string {
	result := ""
	for num > 0 {
		num--
		result = string(rune('A'+num%26)) + result
		num /= 26
	}
	return result
}

func getMoscowTime() time.Time {
	moscowTZ, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Printf("Error loading Moscow timezone: %v, falling back to UTC", err)
		moscowTZ = time.UTC
	}
	return time.Now().In(moscowTZ)
}

func getMoscowLocation() *time.Location {
	moscowTZ, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Printf("Error loading Moscow timezone: %v, falling back to UTC", err)
		return time.UTC
	}
	return moscowTZ
}
