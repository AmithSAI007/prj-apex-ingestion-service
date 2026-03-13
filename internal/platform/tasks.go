package platform

import (
	"context"

	"cloud.google.com/go/cloudtasks/apiv2"
)

// CloudTask wraps the Cloud Tasks SDK client for shared use across the application.
type CloudTask struct {
	client *cloudtasks.Client
}

// NewCloudTask initializes a Cloud Tasks client with ADC and returns a wrapped CloudTask instance.
func NewCloudTask(ctx context.Context) (*CloudTask, error) {

	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &CloudTask{
		client: client,
	}, nil

}

// Client returns the underlying Cloud Tasks client for direct use.
func (c *CloudTask) Client() *cloudtasks.Client {
	return c.client
}

// Close releases resources held by the Cloud Tasks client.
func (c *CloudTask) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
