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
