output "apex_video_ingestion_subscription_name" {
  description = "The name of the Pub/Sub subscription to monitor for new video upload notifications."
  value       = data.google_pubsub_subscription.apex_video_ingestion_subscription.name
}
