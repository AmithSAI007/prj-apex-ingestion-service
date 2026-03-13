data "google_storage_bucket" "apex_dev_gcs_raw_videos" {
  name = var.raw_videos_bucket_name
}
