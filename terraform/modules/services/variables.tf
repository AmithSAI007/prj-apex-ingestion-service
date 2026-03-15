variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "project_region" {
  type        = string
  description = "The GCP region where the resources will be deployed, impacting latency and compliance."
  validation {
    condition     = length(var.project_region) > 0
    error_message = "The project_region must be specified."
  }
}

variable "transcoder_service_name" {
  description = "The name of the Cloud Run service."
  type        = string
  default     = "prj-apex-transcode-submitter"
}

variable "service_name" {
  description = "The name of the Cloud Run service."
  type        = string
  default     = "prj-apex-ingestion-service"
}

variable "container_image" {
  description = "The container image to be used for the Cloud Run service."
  type        = string
}

variable "service_account_name" {
  description = "The service account name to be used by the Cloud Run service."
  type        = string
}

variable "min_instance_count" {
  description = "The minimum number of instances for the Cloud Run service."
  type        = number
}

variable "max_instance_count" {
  description = "The maximum number of instances for the Cloud Run service."
  type        = number
}

variable "memory_limit" {
  description = "The memory limit for the Cloud Run service."
  type        = string
}

variable "cpu_limit" {
  description = "The CPU limit for the Cloud Run service."
  type        = string
}

variable "http_port" {
  description = "The HTTP port for the Cloud Run service."
  type        = string
  default     = ":8080"
}

variable "gcs_bucket" {
  description = "The GCS bucket where uploaded objects are stored."
  type        = string
}

variable "otel_service_name" {
  description = "The logical service name reported to the OpenTelemetry collector."
  type        = string
  default     = "prj-apex-ingestion-service"
}

variable "max_file_size_bytes" {
  description = "Maximum allowed file size in bytes."
  type        = number
  default     = 500000000
}

variable "min_file_size_bytes" {
  description = "Minimum allowed file size in bytes."
  type        = number
  default     = 1000000
}

variable "magic_byte_header_size" {
  description = "Number of bytes to read for magic byte validation."
  type        = number
  default     = 12
}

variable "allowed_video_formats" {
  description = "List of allowed video formats for validation."
  type        = list(string)
  default     = ["mp4", "avi", "mkv"]
}

variable "cloud_tasks_queue_path" {
  description = "The full path to the Cloud Tasks queue."
  type        = string
}

variable "cloud_tasks_queue_name" {
  description = "The name of the Cloud Tasks queue."
  type        = string
}

variable "pubsub_subscription_id" {
  description = "The Pub/Sub subscription ID for receiving events."
  type        = string
}

variable "max_outstanding_messages" {
  description = "Maximum number of outstanding messages for Pub/Sub."
  type        = number
  default     = 10
}

variable "otel_exporter_otlp_endpoint" {
  description = "The endpoint for the OpenTelemetry Protocol (OTLP) exporter."
  type        = string
}

variable "otel_exporter_otlp_headers" {
  description = "Headers for the OTLP exporter, formatted as 'key1=value1,key2=value2'."
  type        = string
}

variable "otel_resource_attributes" {
  description = "Resource attributes for OpenTelemetry, formatted as 'key1=value1,key2=value2'."
  type        = string
}

variable "firestore_database_id" {
  description = "The Firestore database ID to be used by the service."
  type        = string
}
