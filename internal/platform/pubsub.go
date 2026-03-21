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

type MessageHandler func(ctx context.Context, messageId string, data []byte, attrs map[string]string) Result

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
	s.logger.Info("Starting Pub/Sub subscriber",
		zap.String("component", "platform.pubsub"),
		zap.String("action", "start_subscriber"),
		zap.String("subscription", s.subscription.ID()))

	return s.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		s.logger.Info("Message received",
			zap.String("component", "platform.pubsub"),
			zap.String("action", "receive_message"),
			zap.String("messageId", msg.ID),
			zap.Int("dataSize", len(msg.Data)),
			zap.Time("publishTime", msg.PublishTime))

		result := handler(ctx, msg.ID, msg.Data, msg.Attributes)
		switch result {
		case ResultAck:
			msg.Ack()
			s.logger.Info("Message acknowledged",
				zap.String("component", "platform.pubsub"),
				zap.String("action", "ack_message"),
				zap.String("outcome", "ack"),
				zap.String("messageId", msg.ID))
		case ResultNack:
			msg.Nack()
			s.logger.Warn("Message negatively acknowledged",
				zap.String("component", "platform.pubsub"),
				zap.String("action", "nack_message"),
				zap.String("outcome", "nack"),
				zap.String("messageId", msg.ID))
		}
	})

}

func (s *Subscriber) Close() error {
	s.logger.Info("Closing Pub/Sub subscriber",
		zap.String("component", "platform.pubsub"),
		zap.String("action", "close_subscriber"),
		zap.String("subscription", s.subscription.ID()))
	return s.client.Close()
}
