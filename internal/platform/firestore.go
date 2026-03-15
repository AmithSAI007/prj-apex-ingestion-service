package platform

import (
	"context"

	"cloud.google.com/go/firestore"
)

// Client wraps the Firestore SDK client for shared use across the application.
type Client struct {
	client *firestore.Client
}

// NewClient initializes a Firestore client with ADC and returns a wrapped Client instance.
func NewClient(ctx context.Context, projectID, databaseID string) (*Client, error) {
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, err
	}
	return &Client{
		client: client,
	}, nil
}

// Client returns the underlying Firestore client for direct use.
func (c *Client) Client() *firestore.Client {
	return c.client
}

// Close releases resources held by the Firestore client.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
