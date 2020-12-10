FROM alpine
RUN apk update && apk add -u --no-cache git curl unzip tar tini bash && \
    adduser -D harvester && su -l harvester && \
    mkdir -p /var/lib/rancher/harvester && \
    chown -R harvester /var/lib/rancher/harvester /usr/local/bin

WORKDIR /var/lib/rancher/harvester

ENV HARVESTER_UI_VERSION v0.1.1
ENV HARVESTER_UI_PATH /usr/share/rancher/harvester
# Please update the api-ui-version in pkg/settings/settings.go when updating the version here.
ENV HARVESTER_API_UI_VERSION 1.1.9

ARG VERSION=dev
ENV HARVESTER_SERVER_VERSION ${VERSION}
RUN mkdir -p /usr/share/rancher/harvester && \
    cd /usr/share/rancher/harvester && \
    curl -sL https://releases.rancher.com/harvester-ui/${HARVESTER_UI_VERSION}.tar.gz | tar xvzf - --strip-components=2 && \
    mkdir -p /usr/share/rancher/harvester/api-ui && \
    cd /usr/share/rancher/harvester/api-ui && \
    curl -sL https://releases.rancher.com/api-ui/${HARVESTER_API_UI_VERSION}.tar.gz | tar xvzf - --strip-components=1 && \
    cd /var/lib/rancher/harvester

COPY entrypoint.sh harvester /usr/bin/
RUN chmod +x /usr/bin/entrypoint.sh

VOLUME /var/lib/rancher/harvester
ENTRYPOINT ["entrypoint.sh"]
