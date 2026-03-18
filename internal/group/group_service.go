package group

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Store is the minimal storage interface needed by Service.
// It is satisfied by storage.Store without creating an import cycle.
type Store interface {
	CreateApp(ctx context.Context, a *App) error
}

// Service provides business logic operations on top of raw CRUD methods.
type Service struct {
	store  Store
	encKey []byte
}

// NewService creates a Service using the given store and 32-byte AES key.
func NewService(store Store, encKey []byte) *Service {
	return &Service{store: store, encKey: encKey}
}

// EncryptSecret encrypts a plaintext secret using the service's AES key.
func (s *Service) EncryptSecret(plaintext string) (string, error) {
	return EncryptSecret(plaintext, s.encKey)
}

// DecryptSecret decrypts a hex-encoded ciphertext using the service's AES key.
func (s *Service) DecryptSecret(cipherHex string) (string, error) {
	return DecryptSecret(cipherHex, s.encKey)
}

// CreateApp builds a full App record: generates webhook secret, encrypts
// MinIO credentials, and persists via store.
func (s *Service) CreateApp(
	ctx context.Context,
	groupID, name, description string,
	src, dst MinIOConfig,
) (*App, error) {
	webhookSecret, err := GenerateWebhookSecret()
	if err != nil {
		return nil, err
	}
	encSrcSecret, err := EncryptSecret(src.SecretKey, s.encKey)
	if err != nil {
		return nil, err
	}
	encDstSecret, err := EncryptSecret(dst.SecretKey, s.encKey)
	if err != nil {
		return nil, err
	}

	src.SecretKey = encSrcSecret
	dst.SecretKey = encDstSecret

	app := &App{
		ID:            uuid.NewString(),
		GroupID:       groupID,
		Name:          name,
		Description:   description,
		Src:           src,
		Dst:           dst,
		WebhookSecret: webhookSecret,
		Enabled:       true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := s.store.CreateApp(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}
