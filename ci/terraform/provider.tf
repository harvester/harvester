terraform {
  required_version = ">= 0.13"
  required_providers {
    harvester = {
      source  = "harvester/harvester"
      version = "1.7.1"
    }
  }
}

provider "harvester" {
  kubeconfig = var.common.kubeconf
}
