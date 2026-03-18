package transfer

import (
	"errors"

	"github.com/ali/flowgate/internal/group"
	"github.com/ali/flowgate/internal/storage"
)

// TransferStatus represents the lifecycle state of a single object transfer.
type TransferStatus = string

const (
	StatusPending    TransferStatus = "pending"
	StatusInProgress TransferStatus = "in_progress"
	StatusSuccess    TransferStatus = "success"
	StatusFailed     TransferStatus = "failed"
)

// TransferJob is the unit of work placed on the jobs channel.
type TransferJob struct {
	Transfer  *storage.Transfer
	App       group.App
	ObjectKey string
}

// ErrQueueFull is returned by Manager.Enqueue when the jobs channel is at capacity.
var ErrQueueFull = errors.New("transfer queue is full")
