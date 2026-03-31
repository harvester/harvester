ARG HARVESTER_INSTALLER_REF=master-head
FROM registry.opensuse.org/isv/rancher/harvester/os/dev/main/baseos:latest as baseos
FROM rancher/harvester-installer:${HARVESTER_INSTALLER_REF} as installer
FROM registry.suse.com/bci/bci-base:16.0

ARG ARCH=amd64
ENV ARCH=${ARCH}

ARG KUBECTL_VERSION=v1.33.7
ARG KUBECTL_SHA256_amd64=471d94e208a89be62eb776700fc8206cbef11116a8de2dc06fc0086b0015375b
ARG KUBECTL_SHA256_arm64=fa7ee98fdb6fba92ae05b5e0cde0abd5972b2d9a4a084f7052a1fd0dce6bc1de

ARG KUBEVIRT_VERSION=v1.7.1
ARG VIRTCTL_SHA256_amd64=e0efcfc708067fa45232f3bab9cb2de3dbcd812d4c9aab88c727025fb213079f
ARG VIRTCTL_SHA256_arm64=7737d967bc8512abedfdaa8a61a3512f93894c12162f9dde4fab73402a4f42d5

ARG YQ_VERSION=v4.52.4
ARG YQ_SHA256_amd64=0c4d965ea944b64b8fddaf7f27779ee3034e5693263786506ccd1c120f184e8c
ARG YQ_SHA256_arm64=4c2cc022a129be5cc1187959bb4b09bebc7fb543c5837b93001c68f97ce39a5d

ARG WHARFIE_VERSION=v0.7.0
ARG WHARFIE_SHA256_amd64=e5ff747b2f9f4155ce7b68917bac9dbe8a6a85727a94b0c8e6faca9252931e91
ARG WHARFIE_SHA256_arm64=b8e02fe61d4f8cb1bd7927fd5e34b49b4dcf802c52670adfaa4527ed3d9afc41

SHELL ["/bin/bash", "-c"]
RUN zypper rm -y container-suseconnect && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl e2fsprogs rsync awk zstd jq helm zip unzip nginx util-linux && zypper clean -a

ENV KUBECTL_VERSION ${KUBECTL_VERSION}
RUN KUBECTL_SHA256=KUBECTL_SHA256_${ARCH} && \
    VIRTCTL_SHA256=VIRTCTL_SHA256_${ARCH} && \
    YQ_SHA256=YQ_SHA256_${ARCH} && \
    WHARFIE_SHA256=WHARFIE_SHA256_${ARCH} && \
    curl -sfL https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl > /usr/bin/kubectl && \
    echo "${!KUBECTL_SHA256} /usr/bin/kubectl" | sha256sum -c - && \
    chmod +x /usr/bin/kubectl && \
    curl -sfL https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/virtctl-${KUBEVIRT_VERSION}-linux-${ARCH} -o /usr/bin/virtctl && \
    echo "${!VIRTCTL_SHA256} /usr/bin/virtctl" | sha256sum -c - && \
    chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH} -o /usr/bin/yq && \
    echo "${!YQ_SHA256} /usr/bin/yq" | sha256sum -c - && \
    chmod +x /usr/bin/yq && \
    curl -sfL https://github.com/rancher/wharfie/releases/download/${WHARFIE_VERSION}/wharfie-${ARCH}  -o /usr/bin/wharfie && \
    echo "${!WHARFIE_SHA256} /usr/bin/wharfie" | sha256sum -c - && \
    chmod +x /usr/bin/wharfie
# Copy elemental binary to be used for upgrades
COPY --from=baseos /usr/bin/elemental /usr/local/bin/elemental
# Copy harvester-installer binary to be used to generate networkmanager config
COPY --from=installer /usr/bin/harvester-installer /usr/local/bin/harvester-installer
COPY do_upgrade_node.sh /usr/local/bin/
COPY upgrade_node.sh /usr/local/bin/
COPY upgrade_manifests.sh /usr/local/bin/
COPY lib.sh /usr/local/bin
COPY extra_manifests /usr/local/share/extra_manifests
COPY migrations /usr/local/share/migrations
COPY addons /usr/local/share/addons
COPY upgrade-helper /usr/local/bin/
