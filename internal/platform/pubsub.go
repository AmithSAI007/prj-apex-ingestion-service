package platform

import (
	"context"

	"cloud.google.com/go/pubsub/v2"
	"go.uber.org/zap"
)

type Result int

const (
	ResultAck Result = iota
	ResultNack
)

type MessageHandler func(ctx context.Context, data []byte, attrs map[string]string) Result

type Subscriber struct {
	logger       *zap.Logger
	client       *pubsub.Client
	subscription *pubsub.Subscriber
}

func NewSubscriber(ctx context.Context, logger *zap.Logger, projectID, subscriptionID string, maxOutstanding int) (*Subscriber, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	sub := client.Subscriber(subscriptionID)
	sub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxOutstandingMessages: maxOutstanding,
		NumGoroutines:          10,
	}

	return &Subscriber{
		client:       client,
		subscription: sub,
		logger:       logger,
	}, nil
}

func (s *Subscriber) Start(ctx context.Context, handler MessageHandler) error {
	s.logger.Info("Starting Pub/Sub subscriber", zap.String("subscription", s.subscription.ID()))

	return s.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		result := handler(ctx, msg.Data, msg.Attributes)
		switch result {
		case ResultAck:
			msg.Ack()
		case ResultNack:
			msg.Nack()
		}
	})

}

func (s *Subscriber) Close() error {
	s.logger.Info("Closing Pub/Sub subscriber", zap.String("subscription", s.subscription.ID()))
	return s.client.Close()
}
