FROM registry.suse.com/bci/bci-base:16.0

# nfs-client is needed by the dep https://github.com/longhorn/backupstore to check backup store availability.
RUN zypper -n rm container-suseconnect && \
    zypper -n install curl gzip tar nfs-client util-linux && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    useradd -M harvester && \
    mkdir -p /var/lib/harvester/harvester && \
    chown -R harvester /var/lib/harvester/harvester /usr/local/bin

WORKDIR /var/lib/harvester/harvester

ARG HARVESTER_UI_VERSION
ENV HARVESTER_UI_VERSION=${HARVESTER_UI_VERSION:-v1.8.0}
ENV HARVESTER_UI_PATH /usr/share/harvester/harvester
# Please update the api-ui-version in pkg/settings/settings.go when updating the version here.
ENV HARVESTER_API_UI_VERSION 1.1.9

ARG HARVESTER_UI_PLUGIN_BUNDLED_VERSION
ENV HARVESTER_UI_PLUGIN_BUNDLED_VERSION=${HARVESTER_UI_PLUGIN_BUNDLED_VERSION:-v1.8.0}

ARG ARCH=amd64
ARG VERSION=dev
ENV HARVESTER_SERVER_VERSION ${VERSION}
ENV TINI_VERSION v0.19.0
ENV TINI_URL_amd64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini \
    TINI_URL_amd64_sha256=93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c \
    TINI_URL_arm64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-arm64 \
    TINI_URL_arm64_sha256=07952557df20bfd2a95f9bef198b445e006171969499a1d361bd9e6f8e5e0e81 \
    TINI_URL=TINI_URL_${ARCH} \
    TINI_URL_sha256=TINI_URL_${ARCH}_sha256

RUN curl -sLf ${!TINI_URL} > /usr/bin/tini && echo "${!TINI_URL_sha256}  /usr/bin/tini" | sha256sum -c - && chmod +x /usr/bin/tini

RUN mkdir -p /usr/share/harvester/harvester && \
    cd /usr/share/harvester/harvester && \
    curl -sL https://releases.rancher.com/harvester-ui/dashboard/${HARVESTER_UI_VERSION}.tar.gz | tar xvzf - --strip-components=2 && \
    mkdir -p /usr/share/harvester/harvester/api-ui && \
    cd /usr/share/harvester/harvester/api-ui && \
    curl -sL https://releases.rancher.com/api-ui/${HARVESTER_API_UI_VERSION}.tar.gz | tar xvzf - --strip-components=1 && \
    mkdir -p /usr/share/harvester/harvester/v1/harvester/plugin-assets && \
    cd /usr/share/harvester/harvester/v1/harvester/plugin-assets && \
    curl -sL https://releases.rancher.com/harvester-ui/plugin/harvester-${HARVESTER_UI_PLUGIN_BUNDLED_VERSION}.tar.gz | tar xvzf - --strip-components=1 && \
    cd /var/lib/harvester/harvester

COPY entrypoint.sh harvester /usr/bin/
RUN chmod +x /usr/bin/entrypoint.sh

VOLUME /var/lib/harvester/harvester
ENTRYPOINT ["entrypoint.sh"]
