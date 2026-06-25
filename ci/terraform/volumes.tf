resource "harvester_volume" "vol001" {
  name      = "vol001"
  namespace = "default"

  size = "2Gi"
}
