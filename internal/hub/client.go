package hub

import (
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pongWait     = 60 * time.Second
	pingInterval = 54 * time.Second
	writeWait    = 10 * time.Second
	sendBufSize  = 256
)

// Client represents a single WebSocket connection managed by the Hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan Message
}

// NewClient creates a Client, registers it with the hub, and starts its pumps.
// The caller must NOT write to conn after this call.
func NewClient(h *Hub, conn *websocket.Conn) {
	c := &Client{
		hub:  h,
		conn: conn,
		send: make(chan Message, sendBufSize),
	}
	h.Register(c)
	go c.writePump()
	go c.readPump()
}

// readPump reads from the WebSocket to detect disconnection and handle pong frames.
// When the connection closes it unregisters the client from the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Debug("websocket read error", "error", err)
			}
			return
		}
	}
}

// writePump drains the send channel and writes JSON messages to the WebSocket.
// It also sends periodic pings to keep the connection alive.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				slog.Debug("websocket write error", "error", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
