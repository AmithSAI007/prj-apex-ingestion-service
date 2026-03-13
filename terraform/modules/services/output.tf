output "apex_ingestion_service_name" {
  description = "The name of the Cloud Run service used for ingestion."
  value       = google_cloud_run_v2_service.apex_ingestion_service.name
}
