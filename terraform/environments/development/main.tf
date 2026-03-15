module "storage" {
  source = "../../modules/storage"
}

module "pubsub" {
  source = "../../modules/pubsub"
}

module "tasks" {
  source         = "../../modules/tasks"
  project_region = var.project_region
}

module "service" {
  source                      = "../../modules/services"
  project_id                  = var.project_id
  project_region              = var.project_region
  container_image             = var.container_image
  service_account_name        = var.service_account_name
  min_instance_count          = var.min_instance_count
  max_instance_count          = var.max_instance_count
  memory_limit                = var.memory_limit
  cpu_limit                   = var.cpu_limit
  gcs_bucket                  = module.storage.raw_videos_bucket_name
  cloud_tasks_queue_path      = module.tasks.queue_path
  cloud_tasks_queue_name      = module.tasks.queue_name
  pubsub_subscription_id      = module.pubsub.apex_video_ingestion_subscription_name
  otel_exporter_otlp_endpoint = var.otel_exporter_otlp_endpoint
  otel_exporter_otlp_headers  = var.otel_exporter_otlp_headers
  otel_resource_attributes    = var.otel_resource_attributes
  firestore_database_id       = var.firestore_database_id
}
