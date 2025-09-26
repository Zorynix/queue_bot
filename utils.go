package main

import (
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
