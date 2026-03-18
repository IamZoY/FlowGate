package group

import "time"

// Group is a logical namespace grouping one or more Apps.
// Its Name is used as a path segment in the webhook URL.
type Group struct {
	ID          string    `json:"id"          db:"id"`
	Name        string    `json:"name"        db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at"  db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"  db:"updated_at"`
}

// App is a single file-transfer pipeline inside a Group.
// It binds one webhook endpoint to one source MinIO bucket (Src)
// and one destination MinIO bucket (Dst).
//
// Src — the MinIO instance that fires webhooks; GetObject is called here.
// Dst — the MinIO instance that receives files; PutObject is called here.
//
// Both sides are fully independent: different endpoints, credentials, and buckets.
type App struct {
	ID            string      `json:"id"          db:"id"`
	GroupID       string      `json:"group_id"    db:"group_id"`
	Name          string      `json:"name"        db:"name"`
	Description   string      `json:"description" db:"description"`
	Src           MinIOConfig `json:"src"`
	Dst           MinIOConfig `json:"dst"`
	WebhookSecret string      `json:"-"           db:"webhook_secret"` // never serialised
	Enabled       bool        `json:"enabled"     db:"enabled"`
	CreatedAt     time.Time   `json:"created_at"  db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"  db:"updated_at"`
}

// MinIOConfig holds all connection details for one MinIO instance.
// SecretKey is stored AES-GCM encrypted and never appears in JSON responses.
type MinIOConfig struct {
	Endpoint  string `json:"endpoint"   db:"endpoint"`
	AccessKey string `json:"access_key" db:"access_key"`
	SecretKey string `json:"-"          db:"secret_key"` // encrypted at rest
	Bucket    string `json:"bucket"     db:"bucket"`
	Region    string `json:"region"     db:"region"`
	UseSSL    bool   `json:"use_ssl"    db:"use_ssl"`
}
