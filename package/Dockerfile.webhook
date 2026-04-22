FROM registry.suse.com/bci/bci-base:16.0

RUN zypper -n rm container-suseconnect && \
    zypper -n install curl nfs-client util-linux && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/*

ARG ARCH=amd64
ENV TINI_VERSION v0.19.0
ENV TINI_URL_amd64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini \
    TINI_URL_amd64_sha256=93dcc18adc78c65a028a84799ecf8ad40c936fdfc5f2a57b1acda5a8117fa82c \
    TINI_URL_arm64=https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-arm64 \
    TINI_URL_arm64_sha256=07952557df20bfd2a95f9bef198b445e006171969499a1d361bd9e6f8e5e0e81 \
    TINI_URL=TINI_URL_${ARCH} \
    TINI_URL_sha256=TINI_URL_${ARCH}_sha256

RUN curl -sLf ${!TINI_URL} > /usr/bin/tini && echo "${!TINI_URL_sha256}  /usr/bin/tini" | sha256sum -c - && chmod +x /usr/bin/tini

COPY entrypoint-webhook.sh  /usr/bin/entrypoint.sh
COPY harvester-webhook /usr/bin/
RUN chmod +x /usr/bin/entrypoint.sh

ENTRYPOINT ["entrypoint.sh"]
