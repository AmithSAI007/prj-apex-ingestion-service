package platform

import (
	"context"

	"cloud.google.com/go/storage"
)

// GCSClient wraps the GCS SDK client for shared use.
type GCSClient struct {
	client *storage.Client
}

// NewGCSClient initializes a GCS client with ADC.
func NewGCSClient(ctx context.Context) (*GCSClient, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &GCSClient{client: client}, nil
}

// Client returns the underlying storage client.
func (c *GCSClient) Client() *storage.Client {
	return c.client
}

// Close releases resources held by the GCS client.
func (c *GCSClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
