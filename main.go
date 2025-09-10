package main

import (
    "encoding/json"
    "log"
    "net/http"
)

type IncomingMessage struct {
    Messages []struct {
        From string `json:"from"`
        Text struct {
            Body string `json:"body"`
        } `json:"text"`
    } `json:"messages"`
}

type OutgoingMessage struct {
    Reply string `json:"reply"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var msg IncomingMessage
    err := json.NewDecoder(r.Body).Decode(&msg)
    if err != nil {
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }

    log.Printf("Received message from %s: %s\n", msg.Messages[0].From, msg.Messages[0].Text.Body)

    response := OutgoingMessage{
        Reply: "Welcome to FeedMe - the first AI chat to feed you!",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func main() {
    http.HandleFunc("/webhook", webhookHandler)

    log.Println("Starting server on :8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}
