package hub

// Hub maintains the set of active WebSocket clients and broadcasts messages
// to all of them. A single goroutine (Run) owns the client map — no locking needed.
type Hub struct {
	clients    map[*Client]struct{}
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
}

// NewHub returns an initialised Hub. Call Run() in a goroutine before use.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run processes register/unregister/broadcast events.
// Must be called in its own goroutine; runs until the broadcast channel is closed.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}

		case msg, ok := <-h.broadcast:
			if !ok {
				return
			}
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client — drop and evict.
					delete(h.clients, c)
					close(c.send)
				}
			}
		}
	}
}

// Broadcast enqueues a message for delivery to all connected clients.
// Safe to call from any goroutine.
func (h *Hub) Broadcast(msg Message) {
	select {
	case h.broadcast <- msg:
	default:
		// Hub broadcast buffer full — drop the message rather than block.
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}
