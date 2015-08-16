// Server-side events handler for Gin.
// Based on work by Kyle L. Jensen
// Source: https://github.com/kljensen/golang-html5-sse-example

package ssehandler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEHandler struct {
	// Create a map of clients, the keys of the map are the channels over
	// which we can push messages to attached clients. (The values are just
	// booleans and are meaningless.)
	clients map[chan string]bool

	// Channel into which new clients can be pushed
	newClients chan chan string

	// Channel into which disconnected clients should be pushed
	defunctClients chan chan string

	// Channel into which messages are pushed to be broadcast out
	messages chan string
}

// Make a new SSEHandler instance.
func NewSSEHandler() *SSEHandler {
	b := &SSEHandler{
		make(map[chan string]bool),
		make(chan (chan string)),
		make(chan (chan string)),
		make(chan string),
	}
	return b
}

// Start handling new and disconnected clients, as well as sending messages to
// all connected clients.
func (b *SSEHandler) HandleEvents() {
	go func() {
		for {
			select {
			case s := <-b.newClients:
				b.clients[s] = true
			case s := <-b.defunctClients:
				delete(b.clients, s)
				close(s)
			case msg := <-b.messages:
				for s, _ := range b.clients {
					s <- msg
				}
			}
		}
	}()
}

// Send out a simple string to all clients.
func (b *SSEHandler) SendString(msg string) {
	b.messages <- msg
}

// Send out a JSON string object to all clients.
func (b *SSEHandler) SendJSON(obj interface{}) {
	tmp, err := json.Marshal(obj)
	if err != nil {
		log.Panic("Error while sending JSON object:", err)
	}
	b.messages <- string(tmp)
}

// Subscribe a new client and start sending out messages to it.
func (b *SSEHandler) Subscribe(c *gin.Context) {
	w := c.Writer
	f, ok := w.(http.Flusher)
	if !ok {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("Streaming unsupported"))
		return
	}

	// Create a new channel, over which we can send this client messages.
	messageChan := make(chan string)
	// Add this client to the map of those that should receive updates
	b.newClients <- messageChan

	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		// Remove this client from the map of attached clients
		b.defunctClients <- messageChan
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		msg, open := <-messageChan
		if !open {
			// If our messageChan was closed, this means that
			// the client has disconnected.
			break
		}

		fmt.Fprintf(w, "data: Message: %s\n\n", msg)
		// Flush the response. This is only possible if the repsonse
		// supports streaming.
		f.Flush()
	}

	c.AbortWithStatus(http.StatusOK)
}
