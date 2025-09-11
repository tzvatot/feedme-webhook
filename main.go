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
	whatsappToken   = os.Getenv("WHATSAPP_TOKEN")        // WhatsApp Cloud API token
	whatsappPhoneID = os.Getenv("WHATSAPP_PHONE_ID")     // Phone number ID
	verifyToken     = os.Getenv("WHATSAPP_VERIFY_TOKEN") // Token for webhook verification
	port            = os.Getenv("PORT")                  // Port for Render deployment
	anthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")     // Anthropic API key
	anthropicModel  = os.Getenv("ANTHROPIC_MODEL")
)

// Incoming message payload structures
type TextContent struct {
	Body string `json:"body"`
}

type Message struct {
	From string      `json:"from"`
	Text TextContent `json:"text"`
}

type WebhookPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []Message `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// WhatsApp API outgoing message payload
type WhatsAppReply struct {
	MessagingProduct string `json:"messaging_product"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		Body string `json:"body"`
	} `json:"text"`
}

// Anthropic Messages API request/response structures
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicResponse struct {
	Content []AnthropicContentBlock `json:"content"`
}

// Webhook verification
func verifyWebhook(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == verifyToken {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(challenge))
		log.Println("Webhook verified successfully")
		return
	}
	http.Error(w, "Forbidden", http.StatusForbidden)
}

// Main webhook handler
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		verifyWebhook(w, r)
	case http.MethodPost:
		var payload WebhookPayload
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("Invalid payload: %v", err)
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		message := extractMessage(payload)
		if message == nil {
			log.Println("No messages in payload")
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Printf("Received message from %s: %s", message.From, message.Text.Body)

		// Process the message with Claude and send reply
		processUserMessage(*message)

		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Extract the first message safely
func extractMessage(payload WebhookPayload) *Message {
	if len(payload.Entry) == 0 {
		return nil
	}
	if len(payload.Entry[0].Changes) == 0 {
		return nil
	}
	if len(payload.Entry[0].Changes[0].Value.Messages) == 0 {
		return nil
	}
	return &payload.Entry[0].Changes[0].Value.Messages[0]
}

// Send a reply back using WhatsApp API
func sendWhatsAppMessage(to, body string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/messages", whatsappPhoneID)

	reply := WhatsAppReply{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
	}
	reply.Text.Body = body

	payload, err := json.Marshal(reply)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
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
	log.Printf("WhatsApp API response: %s", string(respBody))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to send message: %s", resp.Status)
	}
	return nil
}

// Call Anthropic Claude Messages API to get a reply for the user text
func callClaude(userText string) (string, error) {
	if anthropicAPIKey == "" {
		return "", fmt.Errorf("missing ANTHROPIC_API_KEY")
	}

	reqBody := AnthropicRequest{
		Model:     anthropicModel,
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: userText},
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", anthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error: %s - %s", resp.Status, string(respBytes))
	}

	var ar AnthropicResponse
	if err := json.Unmarshal(respBytes, &ar); err != nil {
		return "", err
	}

	// Concatenate any text blocks in the response content
	var replyText string
	for _, block := range ar.Content {
		if block.Type == "text" {
			replyText += block.Text
		}
	}
	if replyText == "" {
		replyText = "(no response)"
	}
	return replyText, nil
}

// Orchestrate processing of a user message: call Claude and send WhatsApp reply
func processUserMessage(msg Message) {
	reply, err := callClaude(msg.Text.Body)
	if err != nil {
		log.Printf("Claude call failed: %v", err)
		reply = "Sorry, I'm having trouble responding right now."
	}
	if err := sendWhatsAppMessage(msg.From, reply); err != nil {
		log.Printf("Error sending reply: %v", err)
	}
}

func main() {
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/webhook", webhookHandler)

	log.Printf("Server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
