FROM registry.suse.com/bci/bci-base:15.3 as stage0
RUN zypper rm -y container-suseconnect && \
    zypper --no-gpg-checks ref && \
    zypper in -y tar gzip curl

RUN curl -sfL https://get.helm.sh/helm-v3.8.1-linux-amd64.tar.gz -o /tmp/helm.tar.gz && tar xzf /tmp/helm.tar.gz -C /tmp && cp /tmp/linux-amd64/helm /usr/bin/

FROM registry.suse.com/bci/bci-base:15.3

ARG ARCH=amd64
ENV ARCH=${ARCH}
RUN zypper rm -y container-suseconnect && \
    zypper ar --priority=200 http://download.opensuse.org/distribution/leap/15.3/repo/oss repo-oss && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl e2fsprogs rsync awk zstd jq && zypper clean -a

ENV KUBECTL_VERSION v1.21.7
RUN curl -sfL https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl > /usr/bin/kubectl && \
    chmod +x /usr/bin/kubectl

RUN curl -sfL https://github.com/kubevirt/kubevirt/releases/download/v0.45.0/virtctl-v0.45.0-linux-${ARCH} -o /usr/bin/virtctl && chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/download/v4.14.1/yq_linux_${ARCH} -o /usr/bin/yq && chmod +x /usr/bin/yq && \
    curl -sfL https://github.com/rancher/wharfie/releases/latest/download/wharfie-amd64  -o /usr/bin/wharfie && chmod +x /usr/bin/wharfie

COPY --from=stage0 /usr/bin/helm /usr/bin/helm

COPY upgrade_node.sh /usr/local/bin/
COPY upgrade_manifests.sh /usr/local/bin/
COPY lib.sh /usr/local/bin
COPY extra_manifests /usr/local/share/extra_manifests
COPY migrations /usr/local/share/migrations
