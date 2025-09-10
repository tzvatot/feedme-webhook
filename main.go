package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Minimal structures to parse incoming WhatsApp webhook payloads
type WebhookPayload struct {
	Object string  `json:"object"`
	Entry  []Entry `json:"entry"`
}

type Entry struct {
	ID      string   `json:"id"`
	Changes []Change `json:"changes"`
}

type Change struct {
	Value Value `json:"value"`
	Field string `json:"field"`
}

type Value struct {
	MessagingProduct string     `json:"messaging_product"`
	Metadata         Metadata   `json:"metadata"`
	Contacts         []Contact  `json:"contacts"`
	Messages         []Message  `json:"messages"`
}

type Metadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Contact struct {
	Profile Profile `json:"profile"`
	WaID    string  `json:"wa_id"`
}

type Profile struct {
	Name string `json:"name"`
}

type Message struct {
	From      string    `json:"from"`
	ID        string    `json:"id"`
	Timestamp string    `json:"timestamp"`
	Text      *TextBody `json:"text,omitempty"`
	Type      string    `json:"type"`
}

type TextBody struct {
	Body string `json:"body"`
}

// Outbound message request to WhatsApp Cloud API
type WhatsAppSendRequest struct {
	MessagingProduct string      `json:"messaging_product"`
	To               string      `json:"to"`
	Type             string      `json:"type"`
	Text             WhatsText   `json:"text"`
}

type WhatsText struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

func main() {
	http.HandleFunc("/webhook", webhookHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("Starting server on %s ...", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleVerify(w, r)
	case http.MethodPost:
		handleIncoming(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /webhook?hub.mode=subscribe&hub.verify_token=...&hub.challenge=...
func handleVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	verifyToken := os.Getenv("VERIFY_TOKEN")
	if mode == "subscribe" && token == verifyToken {
		// respond with challenge
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, challenge)
		return
	}
	http.Error(w, "Forbidden", http.StatusForbidden)
}

// POST /webhook  -- receives events from WhatsApp Cloud API
func handleIncoming(w http.ResponseWriter, r *http.Request) {
	// It is good practice to respond 200 quickly; we'll process synchronously here.
	var p WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		log.Printf("invalid payload: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Always respond 200 to Facebook/WhatsApp webhook to acknowledge receipt
	w.WriteHeader(http.StatusOK)

	// Extract messages and reply
	for _, entry := range p.Entry {
		for _, ch := range entry.Changes {
			val := ch.Value
			phoneNumberID := val.Metadata.PhoneNumberID
			if phoneNumberID == "" {
				// fallback to env var (configure on Render)
				phoneNumberID = os.Getenv("PHONE_NUMBER_ID")
			}
			for _, msg := range val.Messages {
				from := msg.From
				text := "Welcome to FeedMe - the first AI chat to feed you!"
				// fire the reply (log errors but don't change 200)
				if err := sendWhatsAppText(phoneNumberID, from, text); err != nil {
					log.Printf("send message failed: %v", err)
				} else {
					log.Printf("replied to %s via phoneNumberID=%s", from, phoneNumberID)
				}
			}
		}
	}
}

// sendWhatsAppText calls the WhatsApp Cloud API to send a text message
func sendWhatsAppText(phoneNumberID, to, body string) error {
	if phoneNumberID == "" {
		return fmt.Errorf("phoneNumberID is empty")
	}
	token := os.Getenv("WABA_TOKEN")
	if token == "" {
		return fmt.Errorf("WABA_TOKEN not set")
	}

	url := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/messages", phoneNumberID)
	reqBody := WhatsAppSendRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text: WhatsText{
			PreviewURL: false,
			Body:       body,
		},
	}
	bs, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bs))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var respText bytes.Buffer
		_, _ = respText.ReadFrom(resp.Body)
		return fmt.Errorf("whatsapp api returned %d: %s", resp.StatusCode, respText.String())
	}
	return nil
}
