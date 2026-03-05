package demo

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"cloud.google.com/go/storage"
)

// GCSPath represents a parsed Google Cloud Storage path
type GCSPath struct {
	Bucket string
	Key    string
}

// ParseGCSPath parses a gs://bucket/key format path
// Returns (bucket, key) or error if invalid
func ParseGCSPath(path string) (string, string, error) {
	if !strings.HasPrefix(path, "gs://") {
		return "", "", fmt.Errorf("invalid GCS path: must start with gs://")
	}

	// Remove gs:// prefix
	path = strings.TrimPrefix(path, "gs://")

	// Split on first /
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GCS path: must be gs://bucket/key")
	}

	bucket := parts[0]
	key := parts[1]

	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("invalid GCS path: bucket and key cannot be empty")
	}

	return bucket, key, nil
}

// IsGCSPath checks if path is a GCS path (gs://...)
func IsGCSPath(path string) bool {
	return strings.HasPrefix(path, "gs://")
}

// GCSReader implements io.Reader for GCS range reads
type GCSReader struct {
	client *storage.Client
	reader io.ReadCloser
}

// Read implements io.Reader interface
func (gr *GCSReader) Read(p []byte) (n int, err error) {
	return gr.reader.Read(p)
}

// Close closes the reader and GCS client
func (gr *GCSReader) Close() error {
	if err := gr.reader.Close(); err != nil {
		return err
	}
	return gr.client.Close()
}

// NewGCSRangeReader creates a GCS range reader for specified byte range
func NewGCSRangeReader(ctx context.Context, path string, startByte, endByte int64) (*GCSReader, error) {
	bucket, key, err := ParseGCSPath(path)
	if err != nil {
		return nil, err
	}

	// Create GCS client
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	// Open range reader: NewRangeReader(ctx, offset, length)
	// endByte is inclusive, so length = endByte - startByte + 1
	length := endByte - startByte + 1
	reader, err := client.Bucket(bucket).Object(key).NewRangeReader(ctx, startByte, length)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("open GCS range reader for %s (%d-%d): %w", path, startByte, endByte, err)
	}

	return &GCSReader{
		client: client,
		reader: reader,
	}, nil
}

// GetGCSObjectSize fetches the size of a GCS object
func GetGCSObjectSize(ctx context.Context, path string) (int64, error) {
	bucket, key, err := ParseGCSPath(path)
	if err != nil {
		return 0, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("create GCS client: %w", err)
	}
	defer client.Close()

	attrs, err := client.Bucket(bucket).Object(key).Attrs(ctx)
	if err != nil {
		return 0, fmt.Errorf("get GCS object size: %w", err)
	}

	return attrs.Size, nil
}

// WriteGCSFile writes data to a GCS object
func WriteGCSFile(ctx context.Context, path string, data []byte) error {
	bucket, key, err := ParseGCSPath(path)
	if err != nil {
		return err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create GCS client: %w", err)
	}
	defer client.Close()

	writer := client.Bucket(bucket).Object(key).NewWriter(ctx)
	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return fmt.Errorf("write to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close GCS writer: %w", err)
	}

	log.Printf("Successfully wrote %d bytes to %s", len(data), path)
	return nil
}

// ReadGCSFile reads entire content from a GCS object
func ReadGCSFile(ctx context.Context, path string) ([]byte, error) {
	bucket, key, err := ParseGCSPath(path)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	defer client.Close()

	reader, err := client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("open GCS file %s: %w", path, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read GCS file %s: %w", path, err)
	}

	return data, nil
}

// ListGCSObjects lists objects in a GCS directory and returns object keys
func ListGCSObjects(ctx context.Context, basePath string) ([]string, error) {
	bucket, prefix, err := ParseGCSPath(basePath)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	defer client.Close()

	var objects []string
	it := client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})

	for {
		attrs, err := it.Next()
		if err == storage.ErrObjectNotExist {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list GCS objects: %w", err)
		}

		// Return just the object name (relative to prefix)
		objects = append(objects, attrs.Name)
	}

	return objects, nil
}
