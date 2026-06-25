variable common {
  description = "common variables"
  type = object({
    kubeconf = string
  })
  default = {
    kubeconf = "kubeconf/kubeconfig"
  }
}
