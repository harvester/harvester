#!/usr/bin/env bash

# -----------------------------------------------------------------------------
# Kubectl variables helpers. These functions need the
# following variables:
#
#    K8S_VERSION     -  The Kubernetes version for the cluster, default is v1.18.2.

function vm::kubectl::install() {
  local version=${K8S_VERSION:-"v1.18.2"}
  curl -fL "https://storage.googleapis.com/kubernetes-release/release/${version}/bin/$(vm::util::get_os)/$(vm::util::get_arch)/kubectl" -o /tmp/kubectl
  chmod +x /tmp/kubectl && mv /tmp/kubectl /usr/local/bin/kubectl
}

function vm::kubectl::validate() {
  if [[ -n "$(command -v kubectl)" ]]; then
    return 0
  fi

  vm::log::info "installing kubectl"
  if vm::kubectl::install; then
    vm::log::info "kubectl: $(kubectl version --short --client)"
    return 0
  fi
  vm::log::error "no kubectl available"
  return 1
}
