FROM registry.suse.com/bci/golang:1.22 AS builder

WORKDIR /app

# Copy the build script and source code
COPY scripts/build /app/scripts/build
COPY . /app

# Make the build script executable
RUN chmod +x /app/scripts/build

# Run the build script
RUN /app/scripts/build

FROM registry.suse.com/bci/bci-base:15.6

ARG ARCH=amd64
ENV ARCH=${ARCH}
RUN zypper rm -y container-suseconnect && \
    zypper --no-gpg-checks ref && \
    zypper in -y curl e2fsprogs rsync awk zstd jq helm zip unzip nginx && zypper clean -a

ENV KUBECTL_VERSION v1.29.9
RUN curl -sfL https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl > /usr/bin/kubectl && \
    chmod +x /usr/bin/kubectl

RUN curl -sfL https://github.com/kubevirt/kubevirt/releases/download/v1.3.1/virtctl-v1.3.1-linux-${ARCH} -o /usr/bin/virtctl && chmod +x /usr/bin/virtctl && \
    curl -sfL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${ARCH} -o /usr/bin/yq && chmod +x /usr/bin/yq && \
    curl -sfL https://github.com/rancher/wharfie/releases/download/v0.6.8/wharfie-${ARCH}  -o /usr/bin/wharfie && chmod +x /usr/bin/wharfie

COPY package/upgrade/do_upgrade_node.sh /usr/local/bin/
COPY package/upgrade/upgrade_node.sh /usr/local/bin/
COPY package/upgrade/upgrade_manifests.sh /usr/local/bin/
COPY package/upgrade/lib.sh /usr/local/bin
COPY package/upgrade/patch_99_custom.sh /usr/local/bin
COPY package/upgrade/extra_manifests /usr/local/share/extra_manifests
COPY package/upgrade/migrations /usr/local/share/migrations
COPY package/upgrade/addons /usr/local/share/addons

# Copy the built binaries from the builder stage
COPY --from=builder /app/bin/upgrade-helper /usr/local/bin/harvester-webhook
