FROM alpine
RUN apk update && apk add -u --no-cache git curl unzip tar tini bash nfs-utils && \
    adduser -D harvester && su -l harvester && \
    mkdir -p /var/lib/harvester/harvester && \
    chown -R harvester /var/lib/harvester/harvester /usr/local/bin

WORKDIR /var/lib/harvester/harvester

ENV HARVESTER_UI_VERSION v1.0.0-rc1
ENV HARVESTER_UI_PATH /usr/share/harvester/harvester
# Please update the api-ui-version in pkg/settings/settings.go when updating the version here.
ENV HARVESTER_API_UI_VERSION 1.1.9

ARG VERSION=dev
ENV HARVESTER_SERVER_VERSION ${VERSION}
RUN mkdir -p /usr/share/harvester/harvester && \
    cd /usr/share/harvester/harvester && \
    curl -sL https://releases.rancher.com/harvester-ui/dashboard/${HARVESTER_UI_VERSION}.tar.gz | tar xvzf - --strip-components=2 && \
    mkdir -p /usr/share/harvester/harvester/api-ui && \
    cd /usr/share/harvester/harvester/api-ui && \
    curl -sL https://releases.rancher.com/api-ui/${HARVESTER_API_UI_VERSION}.tar.gz | tar xvzf - --strip-components=1 && \
    cd /var/lib/harvester/harvester

COPY entrypoint.sh harvester /usr/bin/
RUN chmod +x /usr/bin/entrypoint.sh

VOLUME /var/lib/harvester/harvester
ENTRYPOINT ["entrypoint.sh"]
