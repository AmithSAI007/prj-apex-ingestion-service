variable "video_ingestion_subscription_name" {
  description = "The name of the Pub/Sub subscription to monitor for new video upload notifications."
  type        = string
  default     = "apex.video-ingestion.gcs.object-finalized.ingestion-service"
}
