output "queue_name" {
  value = google_cloud_tasks_queue.queue.name
}

output "queue_path" {
  value = google_cloud_tasks_queue.queue.path
}
