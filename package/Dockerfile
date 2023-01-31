FROM registry.suse.com/bci/bci-base:15.4

# nfs-client is needed by the dep https://github.com/longhorn/backupstore to check backup store availability.
RUN zypper -n rm container-suseconnect && \
    zypper -n install curl gzip tar nfs-client && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/* && \
    useradd -M harvester && \
    mkdir -p /var/lib/harvester/harvester && \
    chown -R harvester /var/lib/harvester/harvester /usr/local/bin

WORKDIR /var/lib/harvester/harvester

ENV HARVESTER_UI_VERSION latest
ENV HARVESTER_UI_PATH /usr/share/harvester/harvester
# Please update the api-ui-version in pkg/settings/settings.go when updating the version here.
ENV HARVESTER_API_UI_VERSION 1.1.9

ENV HARVESTER_UI_PLUGIN_BUNDLED_VERSION latest

ARG ARCH=amd64
ARG VERSION=dev
ENV HARVESTER_SERVER_VERSION ${VERSION}
ENV TINI_VERSION v0.19.0
ENV TINI_URL_amd64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini \
    TINI_URL_arm64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-arm64 \
    TINI_URL_s390x=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-s390x \
    TINI_URL=TINI_URL_${ARCH}

RUN curl -sLf ${!TINI_URL} > /usr/bin/tini && chmod +x /usr/bin/tini

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
