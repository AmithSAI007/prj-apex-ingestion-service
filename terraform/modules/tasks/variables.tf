variable "project_region" {
  type        = string
  description = "The GCP region where the resources will be deployed, impacting latency and compliance."
  validation {
    condition     = length(var.project_region) > 0
    error_message = "The project_region must be specified."
  }
}

variable "transcoder_queue_name" {
  description = "The name of the Cloud Tasks queue for transcoding tasks."
  type        = string
  default     = "apex-transcoder-service-queue"
}

variable "max_dispatches_per_second" {
  description = "The maximum number of tasks that can be dispatched per second from the Cloud Tasks queue."
  type        = number
  default     = 5
}

variable "max_concurrent_dispatches" {
  description = "The maximum number of tasks that can be dispatched concurrently from the Cloud Tasks queue."
  type        = number
  default     = 30
}

variable "max_attempts" {
  description = "The maximum number of attempts for a task in the Cloud Tasks queue before it is considered failed."
  type        = number
  default     = 5
}

variable "min_backoff" {
  description = "The minimum backoff time (in seconds) between task retries in the Cloud Tasks queue."
  type        = string
  default     = "10s"
}

variable "max_backoff" {
  description = "The maximum backoff time (in seconds) between task retries in the Cloud Tasks queue."
  type        = string
  default     = "600s"
}

variable "max_doublings" {
  description = "The maximum number of times the backoff time will be doubled between retries in the Cloud Tasks queue."
  type        = number
  default     = 5
}
