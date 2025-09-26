package main

type Subject struct {
	Day   string `json:"day"`
	Start string `json:"start"`
	Name  string `json:"name"`
	End   string `json:"end"`
}

type UserMapping struct {
	TelegramUsername string `json:"TelegramUsername"`
	RealName         string `json:"RealName"`
}
