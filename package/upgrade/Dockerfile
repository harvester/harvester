FROM registry.opensuse.org/isv/rancher/harvester/os/dev/main/baseos:latest as baseos
FROM registry.suse.com/bci/bci-base:16.0

ARG ARCH=amd64
ENV ARCH=${ARCH}

ARG KUBECTL_OBS_PACKAGE=kubectl1.35
ARG HELM_OBS_PACKAGE=helm3

ARG KUBEVIRT_VERSION=v1.7.1
ARG VIRTCTL_SHA256_amd64=e0efcfc708067fa45232f3bab9cb2de3dbcd812d4c9aab88c727025fb213079f
ARG VIRTCTL_SHA256_arm64=7737d967bc8512abedfdaa8a61a3512f93894c12162f9dde4fab73402a4f42d5

ARG WHARFIE_VERSION=v0.7.0
ARG WHARFIE_SHA256_amd64=e5ff747b2f9f4155ce7b68917bac9dbe8a6a85727a94b0c8e6faca9252931e91
ARG WHARFIE_SHA256_arm64=b8e02fe61d4f8cb1bd7927fd5e34b49b4dcf802c52670adfaa4527ed3d9afc41

SHELL ["/bin/bash", "-c"]

RUN zypper addrepo --refresh https://download.opensuse.org/repositories/isv:/Rancher:/dependencies/pkgs rancher-obs-deps && \
    zypper --gpg-auto-import-keys ref --force && \
    zypper rm -y container-suseconnect && \
    zypper in -y curl e2fsprogs rsync awk zstd jq zip unzip nginx util-linux && \
    zypper in -y --from rancher-obs-deps yq ${HELM_OBS_PACKAGE} ${KUBECTL_OBS_PACKAGE} && \
    zypper clean -a && \
    ln -s /usr/bin/${KUBECTL_OBS_PACKAGE} /usr/bin/kubectl && \
    ln -s /usr/bin/${HELM_OBS_PACKAGE} /usr/bin/helm

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
