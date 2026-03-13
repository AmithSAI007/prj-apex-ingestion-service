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
  source                 = "../../modules/service"
  project_region         = var.project_region
  container_image        = var.container_image
  service_account_name   = var.service_account_name
  min_instance_count     = var.min_instance_count
  max_instance_count     = var.max_instance_count
  memory_limit           = var.memory_limit
  cpu_limit              = var.cpu_limit
  gcs_bucket             = module.storage.bucket_name
  cloud_tasks_queue_path = module.tasks.queue_path
  cloud_tasks_queue_name = module.tasks.queue_name
  pubsub_subscription_id = module.pubsub.subscription_id
}
