package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	id   string
	conn *websocket.Conn
}

var (
	clients   = make(map[string]*Client)
	clientsMu sync.RWMutex
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	// First message must be registration: {"type":"register", "id":"clientID"}
	var regMsg map[string]string
	if err := conn.ReadJSON(&regMsg); err != nil || regMsg["type"] != "register" || regMsg["id"] == "" {
		conn.WriteJSON(map[string]string{"type": "error", "message": "Must register with an id"})
		return
	}
	id := regMsg["id"]
	client := &Client{id: id, conn: conn}

	clientsMu.Lock()
	if _, exists := clients[id]; exists {
		clientsMu.Unlock()
		conn.WriteJSON(map[string]string{"type": "error", "message": "ID already registered"})
		return
	}
	clients[id] = client
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, id)
		clientsMu.Unlock()
	}()

	log.Printf("Client registered: %s", id)
	conn.WriteJSON(map[string]string{"type": "registered", "id": id})

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Client %s disconnected: %v", id, err)
			break
		}
		// Expecting {"type": "signal", "to": "otherID", ...}
		if msg["type"] == "signal" && msg["to"] != nil {
			toID := msg["to"].(string)
			clientsMu.RLock()
			toClient, ok := clients[toID]
			clientsMu.RUnlock()
			if ok {
				msg["from"] = id
				if err := toClient.conn.WriteJSON(msg); err != nil {
					log.Printf("Error relaying to %s: %v", toID, err)
				}
			} else {
				conn.WriteJSON(map[string]string{"type": "error", "message": "Target not found"})
			}
		} else {
			conn.WriteJSON(map[string]string{"type": "error", "message": "Invalid message"})
		}
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	addr := ":8080"
	fmt.Println("WebRTC broker listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
