terraform {
  backend "gcs" {
    bucket = "prj-apex-infra-terraform-state"
    prefix = "terraform/apex-ingestion-service/state"
  }
}
