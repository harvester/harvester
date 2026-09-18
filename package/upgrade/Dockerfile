FROM registry.opensuse.org/isv/rancher/harvester/os/v1.9/main/baseos:v1.9 as baseos
FROM registry.suse.com/bci/bci-base:16.0

ARG ARCH=amd64
ENV ARCH=${ARCH}

ARG KUBECTL_VERSION=v1.36.4
ARG KUBECTL_SHA256_amd64=8b8f088da2dab964f853b38464033b1be15ede2839eca751482357c45abdd05a
ARG KUBECTL_SHA256_arm64=0ecf44450ee6063bf19dd166a103ee6df4a9034455c2abce626e6eea657d73fb

ARG KUBEVIRT_VERSION=v1.8.3
ARG VIRTCTL_SHA256_amd64=91855c13ffcc279f13ab47772fd23074855914c7b6f88af306c87a7089bc6536
ARG VIRTCTL_SHA256_arm64=5cab0598c56afdfea6051c980f26d3859b1b33dc036d3df81d814d67efb00ef9

ARG YQ_VERSION=v4.53.3
ARG YQ_SHA256_amd64=fa52a4e758c63d38299163fbdd1edfb4c4963247918bf9c1c5d31d84789eded4
ARG YQ_SHA256_arm64=578648e463a11c1b6db6010cbf41eafed6bee79466fcffa1bb446672cf7945ea

ARG WHARFIE_VERSION=v0.7.1
ARG WHARFIE_SHA256_amd64=72967d44b588484d3bc2e025b98e98d7b9ae5dd9e8aad20cd8b03699052683ec
ARG WHARFIE_SHA256_arm64=18964c85ed93f8f6d85b7aacd25e1429659be9e6be681d600696c352d9991164

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
    echo "${!KUBECTL_SHA256}  /usr/bin/kubectl" | sha256sum -c - && \
    chmod +x /usr/bin/kubectl && \
    curl -sfL https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/virtctl-${KUBEVIRT_VERSION}-linux-${ARCH} -o /usr/bin/virtctl && \
    echo "${!VIRTCTL_SHA256}  /usr/bin/virtctl" | sha256sum -c - && \
    chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH} -o /usr/bin/yq && \
    echo "${!YQ_SHA256}  /usr/bin/yq" | sha256sum -c - && \
    chmod +x /usr/bin/yq && \
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
