resource "google_cloud_tasks_queue" "transcoder_task_queue" {
  name     = var.transcoder_queue_name
  location = var.project_region
  rate_limits {
    max_dispatches_per_second = var.max_dispatches_per_second
    max_concurrent_dispatches = var.max_concurrent_dispatches
  }

  retry_config {
    max_attempts  = var.max_attempts
    min_backoff   = var.min_backoff
    max_backoff   = var.max_backoff
    max_doublings = var.max_doublings
  }
}
