FROM registry.suse.com/bci/bci-base:15.6

ARG ARCH=amd64
ENV ARCH=${ARCH}
RUN zypper rm -y container-suseconnect && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl e2fsprogs rsync awk zstd jq helm zip unzip nginx && zypper clean -a

ENV KUBECTL_VERSION v1.29.9
RUN curl -sfL https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl > /usr/bin/kubectl && \
    chmod +x /usr/bin/kubectl

RUN curl -sfL https://github.com/kubevirt/kubevirt/releases/download/v1.2.2/virtctl-v1.2.2-linux-${ARCH} -o /usr/bin/virtctl && chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${ARCH} -o /usr/bin/yq && chmod +x /usr/bin/yq && \
    curl -sfL https://github.com/rancher/wharfie/releases/download/v0.6.6/wharfie-${ARCH}  -o /usr/bin/wharfie && chmod +x /usr/bin/wharfie

COPY do_upgrade_node.sh /usr/local/bin/
COPY upgrade_node.sh /usr/local/bin/
COPY upgrade_manifests.sh /usr/local/bin/
COPY lib.sh /usr/local/bin
COPY patch_99_custom.sh /usr/local/bin
COPY extra_manifests /usr/local/share/extra_manifests
COPY migrations /usr/local/share/migrations
COPY addons /usr/local/share/addons
COPY upgrade-helper /usr/local/bin/
