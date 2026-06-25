resource "harvester_virtualmachine" "cirros-01" {
  name                 = "cirros"
  namespace            = "default"
  restart_after_update = true

  description = "test vm on cirros"

  cpu    = 2
  memory = "512Mi"

  run_strategy = "RerunOnFailure"
  hostname     = "vm001"

  network_interface {
    name           = "nic-1"
    wait_for_lease = true
  }

  disk {
    name       = "rootdisk"
    type       = "disk"
    size       = "10Gi"
    bus        = "virtio"
    boot_order = 1

    image       = harvester_image.cirros.id
    auto_delete = true
  }

  disk {
    name        = "disk001"
    type        = "disk"
    size        = "2Gi"
    bus         = "virtio"
    auto_delete = true
  }

  disk {
    name = "mounted-disk"
    type = "disk"
    bus  = "scsi"

    existing_volume_name = harvester_volume.vol001.name
    auto_delete          = false
    hot_plug             = true
  }
}
