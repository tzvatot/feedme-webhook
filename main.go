package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

var (
	// Environment variables
	whatsappToken   = os.Getenv("WHATSAPP_TOKEN")
	whatsappPhoneID = os.Getenv("WHATSAPP_PHONE_ID")
	verifyToken     = os.Getenv("WHATSAPP_VERIFY_TOKEN")
	port            = os.Getenv("PORT")
)

type WhatsAppMessage struct {
	Messages []struct {
		From string `json:"from"`
		Text struct {
			Body string `json:"body"`
		} `json:"text"`
	} `json:"messages"`
}

func sendWhatsAppMessage(to, message string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v16.0/%s/messages", whatsappPhoneID)

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                to,
		"text": map[string]string{
			"body": message,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+whatsappToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("WhatsApp API response: %s", respBody)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to send message, status: %s", resp.Status)
	}

	return nil
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Verification challenge
		mode := r.URL.Query().Get("hub.mode")
		token := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")

		if mode == "subscribe" && token == verifyToken {
			fmt.Fprint(w, challenge)
			return
		} else {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}

		var msg WhatsAppMessage
		err = json.Unmarshal(body, &msg)
		if err != nil {
			http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
			return
		}

		from := msg.Messages[0].From
		userText := msg.Messages[0].Text.Body
		log.Printf("Received message from %s: %s", from, userText)

		replyText := "Welcome to FeedMe - the first AI chat to feed you!"

		err = sendWhatsAppMessage(from, replyText)
		if err != nil {
			log.Printf("Error sending WhatsApp message: %v", err)
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func main() {
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/webhook", webhookHandler)
	log.Printf("Starting server on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
