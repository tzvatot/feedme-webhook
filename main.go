package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const verifyToken = "a67c926c-7c89-438a-a24f-1ebee9cb9b05" // must match the token in Facebook dashboard

type Message struct {
	From string `json:"from"`
	Text struct {
		Body string `json:"body"`
	} `json:"text"`
}

type Incoming struct {
	Messages []Message `json:"messages"`
}

type Reply struct {
	Reply string `json:"reply"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Facebook verification
		mode := r.URL.Query().Get("hub.mode")
		token := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")

		if mode == "subscribe" && token == verifyToken {
			fmt.Fprint(w, challenge)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodPost {
		var incoming Incoming
		err := json.NewDecoder(r.Body).Decode(&incoming)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Log received messages
		fmt.Printf("Received messages: %+v\n", incoming.Messages)

		// Reply with welcome message
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Reply{Reply: "Welcome to FeedMe - the first AI chat to feed you!"})
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func main() {
	http.HandleFunc("/webhook", webhookHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // fallback for local testing
	}

	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Server failed to start: %v\n", err)
		os.Exit(1)
	}
}
