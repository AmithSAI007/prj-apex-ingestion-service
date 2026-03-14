output "queue_name" {
  value = google_cloud_tasks_queue.transcoder_task_queue.name
}

output "queue_path" {
  value = google_cloud_tasks_queue.transcoder_task_queue.id
}
