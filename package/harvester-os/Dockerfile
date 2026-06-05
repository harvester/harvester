ARG BASE_OS_IMAGE
FROM ${BASE_OS_IMAGE}

ARG ARCH=amd64

# renovate: datasource=github-release-attachments depName=rancher/wharfie
ARG WHARFIE_VERSION=v0.6.8
# renovate: datasource=github-release-attachments depName=rancher/wharfie digestVersion=v0.6.8
ARG WHARFIE_SUM_amd64=e6b5d27e5b5815ece828e3d2f4012ccec1e40dceb4e639815d6cdbc0f22e2fa8
# renovate: datasource=github-release-attachments depName=rancher/wharfie digestVersion=v0.6.8
ARG WHARFIE_SUM_arm64=36e50fdd08577cd2adea613141c58830cb73ab4d46ef3f90db62337075518a2d
ENV WHARFIE_SUM="WHARFIE_SUM_${ARCH}"

RUN curl -sfL https://github.com/rancher/wharfie/releases/download/${WHARFIE_VERSION}/wharfie-${ARCH} -o /usr/bin/wharfie \
    && echo "${!WHARFIE_SUM}" /usr/bin/wharfie | sha256sum -c - \
    && chmod +x /usr/bin/wharfie

COPY files/ /
RUN chmod 0600 /system/oem/*

COPY harvester-release.yaml /etc/

RUN dracut -f --regenerate-all

ARG HARVESTER_PRETTY_NAME
RUN sed -i "s/^PRETTY_NAME.*/PRETTY_NAME=\"$HARVESTER_PRETTY_NAME\"/g" /etc/os-release && \
    echo GRUB_ENTRY_NAME="\"$HARVESTER_PRETTY_NAME\"" >> /etc/os-release

