FROM registry.suse.com/bci/bci-base:15.4 AS temp_elemental

RUN zypper rm -y container-suseconnect && \
    zypper ar --priority=200 http://download.opensuse.org/distribution/leap/15.4/repo/oss repo-oss && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl tar gzip && zypper clean -a

RUN mkdir -p /tmp/elemental_cli && \
    curl -sfL https://github.com/rancher/elemental-cli/releases/download/v0.1.1/elemental-v0.1.1-Linux-x86_64.tar.gz | tar -xz -C /tmp/elemental_cli


FROM registry.suse.com/bci/bci-base:15.4

ARG ARCH=amd64
ENV ARCH=${ARCH}
RUN zypper rm -y container-suseconnect && \
    zypper ar --priority=200 http://download.opensuse.org/distribution/leap/15.4/repo/oss repo-oss && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl e2fsprogs rsync awk zstd jq helm zip unzip nginx && zypper clean -a

ENV KUBECTL_VERSION v1.24.11
RUN curl -sfL https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl > /usr/bin/kubectl && \
    chmod +x /usr/bin/kubectl

RUN curl -sfL https://github.com/kubevirt/kubevirt/releases/download/v0.55.2/virtctl-v0.55.2-linux-${ARCH} -o /usr/bin/virtctl && chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${ARCH} -o /usr/bin/yq && chmod +x /usr/bin/yq && \
    curl -sfL https://github.com/rancher/wharfie/releases/latest/download/wharfie-amd64  -o /usr/bin/wharfie && chmod +x /usr/bin/wharfie

COPY --from=temp_elemental /tmp/elemental_cli /usr/local/bin/elemental_cli

COPY do_upgrade_node.sh /usr/local/bin/
COPY upgrade_node.sh /usr/local/bin/
COPY upgrade_manifests.sh /usr/local/bin/
COPY lib.sh /usr/local/bin
COPY patch_99_custom.sh /usr/local/bin
COPY extra_manifests /usr/local/share/extra_manifests
COPY migrations /usr/local/share/migrations
COPY addons /usr/local/share/addons
