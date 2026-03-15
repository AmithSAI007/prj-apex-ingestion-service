data "google_cloud_run_v2_service" "apex_transcoder_service" {
  name     = var.transcoder_service_name
  location = var.project_region
}

resource "google_cloud_run_v2_service" "apex_ingestion_service" {
  name     = var.service_name
  location = var.project_region

  scaling {
    min_instance_count = var.min_instance_count
    max_instance_count = var.max_instance_count
  }

  template {

    service_account = var.service_account_name

    containers {
      image = var.container_image

      resources {
        limits = {
          memory = var.memory_limit
          cpu    = var.cpu_limit
        }
      }

      env {
        name  = "HTTP_PORT"
        value = var.http_port
      }
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "GCP_PROJECT_REGION"
        value = var.project_region
      }
      env {
        name  = "GCS_BUCKET"
        value = var.gcs_bucket
      }
      env {
        name  = "OTEL_SERVICE_NAME"
        value = var.otel_service_name
      }
      env {
        name  = "MAX_FILE_SIZE_BYTES"
        value = var.max_file_size_bytes
      }
      env {
        name  = "MIN_FILE_SIZE_BYTES"
        value = var.min_file_size_bytes
      }
      env {
        name  = "MAGIC_BYTE_HEADER_SIZE"
        value = var.magic_byte_header_size
      }
      env {
        name  = "ALLOWED_VIDEO_FORMATS"
        value = join(",", var.allowed_video_formats)
      }
      env {
        name  = "INVOKER_SERVICE_ACCOUNT_EMAIL"
        value = var.service_account_name
      }
      env {
        name  = "TRANSCODER_SERVICE_URL"
        value = data.google_cloud_run_v2_service.apex_transcoder_service.uri
      }
      env {
        name  = "CLOUD_TASKS_QUEUE_PATH"
        value = var.cloud_tasks_queue_path
      }
      env {
        name  = "CLOUD_TASKS_QUEUE_NAME"
        value = var.cloud_tasks_queue_name
      }
      env {
        name  = "PUBSUB_SUBSCRIPTION_ID"
        value = var.pubsub_subscription_id
      }
      env {
        name  = "MAX_OUTSTANDING_MESSAGES"
        value = var.max_outstanding_messages
      }
    }
  }

  traffic {
    percent = 100
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
  }
}
