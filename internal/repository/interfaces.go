package repository

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/dto"
)

type StorageInterface interface {
	ReadObjectHeader(ctx context.Context, eventId, traceId, userId, videoId, bucket, objectName string, numBytes int64) ([]byte, error)
}

type FirestoreInterface interface {
	TransitionStatus(ctx context.Context, eventId, traceId, userId, videoId, from, to string, updates map[string]interface{}) error
	GetVideoStatus(ctx context.Context, eventId, traceId, userId, videoId string) (string, error)
	Exists(ctx context.Context, eventId, traceId, userId, videoId string) (bool, error)
	docRef(videoId string) *firestore.DocumentRef
}

type CloudTaskInterface interface {
	EnqueueTranscodeTask(ctx context.Context, payload *dto.TranscoderServicePayload) error
}
