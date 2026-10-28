FROM registry.opensuse.org/isv/rancher/harvester/os/dev/main/baseos:latest as baseos
FROM registry.suse.com/bci/bci-base:16.0

ARG ARCH=amd64
ENV ARCH=${ARCH}

ARG KUBECTL_VERSION=1.37

ARG KUBEVIRT_VERSION=v1.9.0
ARG VIRTCTL_SHA256_amd64=40ede2ee37c98a1aeed71c9c219616a05247ce2be109e1edddf0477572e8b978
ARG VIRTCTL_SHA256_arm64=9d9b45d0f09ba2316d255bc629154f293210f803d51ace7995c9f2260999e78e

ARG WHARFIE_VERSION=v0.7.1
ARG WHARFIE_SHA256_amd64=72967d44b588484d3bc2e025b98e98d7b9ae5dd9e8aad20cd8b03699052683ec
ARG WHARFIE_SHA256_arm64=18964c85ed93f8f6d85b7aacd25e1429659be9e6be681d600696c352d9991164

SHELL ["/bin/bash", "-c"]
RUN zypper rm -y container-suseconnect && \
  zypper --no-gpg-checks ref && \
  zypper in -y curl e2fsprogs rsync awk zstd jq helm zip unzip nginx util-linux && zypper clean -a

ENV KUBECTL_VERSION ${KUBECTL_VERSION}
RUN zypper ar -f https://rpm.rancher.io/pkgs/prime pkgs && \
  rpm --import https://raw.githubusercontent.com/rancher/rancher/refs/heads/main/rancher-rpm-pkgs.key && \
  zypper --non-interactive in --no-recommends -r pkgs kubectl-${KUBECTL_VERSION} yq && \
  ln -s /usr/bin/kubectl-${KUBECTL_VERSION} /usr/bin/kubectl

RUN VIRTCTL_SHA256=VIRTCTL_SHA256_${ARCH} && \
  WHARFIE_SHA256=WHARFIE_SHA256_${ARCH} && \
  curl -sfL https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/virtctl-${KUBEVIRT_VERSION}-linux-${ARCH} -o /usr/bin/virtctl && \
  echo "${!VIRTCTL_SHA256}  /usr/bin/virtctl" | sha256sum -c - && \
  chmod +x /usr/bin/virtctl && \
  curl -sfL https://github.com/rancher/wharfie/releases/download/${WHARFIE_VERSION}/wharfie-${ARCH}  -o /usr/bin/wharfie && \
  echo "${!WHARFIE_SHA256}  /usr/bin/wharfie" | sha256sum -c - && \
  chmod +x /usr/bin/wharfie
# Copy elemental binary to be used for upgrades
COPY --from=baseos /usr/bin/elemental /usr/local/bin/elemental
# Copy harvester-installer binary to be used to generate networkmanager config
COPY harvester-installer /usr/local/bin/harvester-installer
COPY do_upgrade_node.sh /usr/local/bin/
COPY upgrade_node.sh /usr/local/bin/
COPY upgrade_manifests.sh /usr/local/bin/
COPY lib.sh /usr/local/bin
COPY extra_manifests /usr/local/share/extra_manifests
COPY migrations /usr/local/share/migrations
COPY addons /usr/local/share/addons
COPY upgrade-helper /usr/local/bin/
