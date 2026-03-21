package dto

type GCSObjectData struct {
	Kind           string            `json:"kind"`
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Bucket         string            `json:"bucket"`
	ContentType    string            `json:"contentType"`
	Size           string            `json:"size"`
	MDFHash        string            `json:"md5Hash"`
	TimeCreated    string            `json:"timeCreated"`
	Updated        string            `json:"updated"`
	StorageClass   string            `json:"storageClass"`
	Generation     string            `json:"generation"`
	Metageneration string            `json:"metageneration"`
	CRC32C         string            `json:"crc32c"`
	Etag           string            `json:"etag"`
	Metadata       map[string]string `json:"metadata"`
}

type MetaData struct {
	EventType string `json:"event_type"`
	Source    string `json:"source"`
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	EventTime string `json:"event_time"`
	TraceID   string `json:"trace_id"`
}

type SuccessResponse struct {
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty" example:"req_6f1a2c9d5e7b3a1c"`
}

type TranscoderServicePayload struct {
	EventID     string `json:"eventId"`
	TraceID     string `json:"traceId"`
	ContentType string `json:"contentType" example:"video/mp4"`
	FileSize    string `json:"fileSize" example:"10485760"`
	VideoID     string `json:"videoId" example:"vid_12345"`
	UserID      string `json:"userId" example:"user_67890"`
	RawFilePath string `json:"rawFilePath" example:"gs://bucket_name/path/to/video.mp4"`
}
