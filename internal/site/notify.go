package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agilepanel/internal/config"
)

func sendTelegramNotification(state *config.State, message string) {
	if state == nil {
		return
	}
	token := state.Global.TelegramBotToken
	chatID := state.Global.TelegramChatID
	if token == "" || chatID == "" {
		return
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(body))
	if err == nil {
		resp.Body.Close()
	}
}

func notifyBackup(domain string, timestamp string, isS3 bool, state *config.State, err error) {
	if state == nil {
		return
	}
	var dest string
	if isS3 {
		dest = "S3 Cloud Storage"
	} else {
		dest = "Local Storage"
	}
	var msg string
	if err != nil {
		msg = fmt.Sprintf("❌ <b>AgilePanel Backup Failed</b>\n\n<b>Domain:</b> %s\n<b>Destination:</b> %s\n<b>Timestamp:</b> %s\n<b>Error:</b> %v", domain, dest, timestamp, err)
	} else {
		msg = fmt.Sprintf("✅ <b>AgilePanel Backup Successful</b>\n\n<b>Domain:</b> %s\n<b>Destination:</b> %s\n<b>Timestamp:</b> %s", domain, dest, timestamp)
	}
	sendTelegramNotification(state, msg)
}
