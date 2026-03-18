package storage

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/ali/flowgate/internal/group"
)

// ObjectStorage abstracts MinIO operations used by the transfer worker.
type ObjectStorage interface {
	GetObject(ctx context.Context, cfg group.MinIOConfig, key string) (io.ReadCloser, int64, error)
	PutObject(ctx context.Context, cfg group.MinIOConfig, key string, r io.Reader, size int64) error
	BucketExists(ctx context.Context, cfg group.MinIOConfig) (bool, error)
}

// MinIOClient implements ObjectStorage, caching minio.Client instances by endpoint+accessKey.
type MinIOClient struct {
	clients sync.Map // key: cacheKey(cfg) → *minio.Client
}

// NewMinIOClient returns a ready-to-use MinIOClient.
func NewMinIOClient() *MinIOClient {
	return &MinIOClient{}
}

func cacheKey(cfg group.MinIOConfig) string {
	return cfg.Endpoint + "|" + cfg.AccessKey
}

func (m *MinIOClient) clientFor(cfg group.MinIOConfig) (*minio.Client, error) {
	k := cacheKey(cfg)
	if v, ok := m.clients.Load(k); ok {
		return v.(*minio.Client), nil
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client for %q: %w", cfg.Endpoint, err)
	}

	// Store-if-absent to avoid races — last writer wins, both are equivalent.
	actual, _ := m.clients.LoadOrStore(k, client)
	return actual.(*minio.Client), nil
}

// GetObject fetches the object at key from the source MinIO and returns a
// streaming ReadCloser plus the object size in bytes.
// The caller is responsible for closing the returned ReadCloser.
func (m *MinIOClient) GetObject(ctx context.Context, cfg group.MinIOConfig, key string) (io.ReadCloser, int64, error) {
	client, err := m.clientFor(cfg)
	if err != nil {
		return nil, 0, err
	}

	obj, err := client.GetObject(ctx, cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("GetObject %q: %w", key, err)
	}

	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, 0, fmt.Errorf("stat %q: %w", key, err)
	}

	return obj, stat.Size, nil
}

// PutObject streams r into the destination MinIO bucket at key.
// size must match the number of bytes in r; pass -1 only when size is unknown.
func (m *MinIOClient) PutObject(ctx context.Context, cfg group.MinIOConfig, key string, r io.Reader, size int64) error {
	client, err := m.clientFor(cfg)
	if err != nil {
		return err
	}

	_, err = client.PutObject(ctx, cfg.Bucket, key, r, size, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("PutObject %q: %w", key, err)
	}
	return nil
}

// BucketExists reports whether the bucket described by cfg exists and is accessible.
func (m *MinIOClient) BucketExists(ctx context.Context, cfg group.MinIOConfig) (bool, error) {
	client, err := m.clientFor(cfg)
	if err != nil {
		return false, err
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return false, fmt.Errorf("BucketExists %q: %w", cfg.Bucket, err)
	}
	return exists, nil
}
