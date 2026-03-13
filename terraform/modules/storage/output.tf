output "raw_videos_bucket_name" {
  description = "The name of the Cloud Storage bucket to monitor for new video uploads."
  value       = data.google_storage_bucket.apex_dev_gcs_raw_videos.name
}
