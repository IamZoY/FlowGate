package hub

import "time"

// MessageType identifies the kind of WebSocket event being broadcast.
type MessageType string

const (
	MsgTransferQueued    MessageType = "transfer:queued"
	MsgTransferStarted   MessageType = "transfer:started"
	MsgTransferCompleted MessageType = "transfer:completed"
	MsgTransferFailed    MessageType = "transfer:failed"
	MsgStatsUpdate       MessageType = "stats:update"
	MsgPing              MessageType = "ping"
	MsgGroupCreated      MessageType = "group:created"
	MsgGroupUpdated      MessageType = "group:updated"
	MsgGroupDeleted      MessageType = "group:deleted"
	MsgAppCreated        MessageType = "app:created"
	MsgAppUpdated        MessageType = "app:updated"
	MsgAppDeleted        MessageType = "app:deleted"
)

// Message is the envelope sent to every connected WebSocket client.
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   any         `json:"payload"`
}
