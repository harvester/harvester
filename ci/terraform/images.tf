resource "harvester_image" "cirros" {
  name      = "cirros"
  namespace = "default"

  display_name = "cirros-0.6.1-x86_64-disk.img"
  source_type  = "download"
  url          = "https://download.cirros-cloud.net/0.6.1/cirros-0.6.1-x86_64-disk.img"
}