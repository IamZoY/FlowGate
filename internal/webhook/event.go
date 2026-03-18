package webhook

// Event is the top-level MinIO S3 notification payload.
type Event struct {
	EventName string   `json:"EventName"`
	Key       string   `json:"Key"`
	Records   []Record `json:"Records"`
}

// Record holds per-event metadata inside the Records array.
type Record struct {
	EventName string `json:"eventName"`
	EventTime string `json:"eventTime"`
	S3        S3Data `json:"s3"`
}

// S3Data carries bucket and object metadata for a single record.
type S3Data struct {
	Bucket S3Bucket `json:"bucket"`
	Object S3Object `json:"object"`
}

// S3Bucket identifies the source bucket.
type S3Bucket struct {
	Name string `json:"name"`
}

// S3Object describes the object that triggered the event.
// Key is URL-encoded; callers must apply url.PathUnescape before use.
type S3Object struct {
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	ETag        string `json:"eTag"`
	ContentType string `json:"contentType"`
}
