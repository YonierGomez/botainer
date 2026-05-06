package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/events"
)

type WSMessage struct {
	Type     string      `json:"type"`
	Resource string      `json:"resource,omitempty"`
	ID       string      `json:"id,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Println("WebSocket client connected")

	// Start Docker events listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsChan, errChan := s.docker.Events(ctx, events.ListOptions{})

	// Send initial ping
	conn.WriteJSON(WSMessage{Type: "connected"})

	// Handle incoming messages and send events
	go func() {
		for {
			var msg WSMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				cancel()
				return
			}

			// Handle ping
			if msg.Type == "ping" {
				conn.WriteJSON(WSMessage{Type: "pong"})
			}
		}
	}()

	// Send Docker events to client
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-eventsChan:
			// Send container events
			if event.Type == "container" {
				conn.WriteJSON(WSMessage{
					Type:     "container_event",
					Resource: "container",
					ID:       event.Actor.ID,
					Data: map[string]interface{}{
						"action": event.Action,
						"time":   event.Time,
						"name":   event.Actor.Attributes["name"],
					},
				})
			}

		case err := <-errChan:
			if err != nil {
				log.Printf("Docker events error: %v", err)
				return
			}

		case <-ticker.C:
			// Send periodic stats update
			// This is a placeholder - implement actual stats collection
			conn.WriteJSON(WSMessage{
				Type: "stats_update",
				Data: map[string]interface{}{
					"timestamp": time.Now().Unix(),
				},
			})

		case <-ctx.Done():
			return
		}
	}
}
