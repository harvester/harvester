FROM golang:1.26-bookworm AS builder

ARG CONTAINER_WORKDIR=/go/src/github.com/harvester/harvester

ARG MK_HOST_ARCH
ENV ARCH=$MK_HOST_ARCH
ENV GOTOOLCHAIN=auto

ARG HELM_VERSION=v3.20.0
ARG HELM_SHA256_Linux_amd64=dbb4c8fc8e19d159d1a63dda8db655f9ffa4aac1b9a6b188b34a40957119b286
ARG HELM_SHA256_Linux_arm64=bfb14953295d5324d47ab55f3dfba6da28d46c848978c8fbf412d4271bdc29f1

ARG KIND_VERSION=v0.31.0
ARG KIND_SHA256_Linux_amd64=eb244cbafcc157dff60cf68693c14c9a75c4e6e6fedaf9cd71c58117cb93e3fa
ARG KIND_SHA256_Linux_arm64=8e1014e87c34901cc422a1445866835d1e666f2a61301c27e722bdeab5a1f7e4

ARG CODECOV_VERSION=v0.8.0
ARG CODECOV_SHA256_Linux=b37359013b48fbc3b0790d59fc474a52a260fb96e28e1b2c2ae001dc9b9cc996

ARG ADDONS_REPO=https://github.com/harvester/addons.git
ARG ADDONS_BRANCH=main

# For common usage, as this path occurs many times in this file
ARG ADDONS_PREPARE_DIR=/dist/prepare-addons

SHELL ["/bin/bash", "-c"]
RUN apt-get update -qq && apt-get install -y --no-install-recommends \
    xz-utils \
    unzip \
    zstd \
    squashfs-tools \
    xorriso \
    jq \
    mtools \
    dosfstools \
    patch \
    && rm -rf /var/lib/apt/lists/*

# install yq
RUN GO111MODULE=on go install github.com/mikefarah/yq/v4@v4.27.5
# set up helm
ENV HELM_VERSION=${HELM_VERSION}
ENV HELM_TARBALL=helm-${HELM_VERSION}-linux-${ARCH}.tar.gz
ENV HELM_URL=https://get.helm.sh/${HELM_TARBALL}
RUN mkdir /usr/tmp && \
    curl -sSLO --output-dir /usr/tmp ${HELM_URL} && \
    HELM_SHA256=HELM_SHA256_Linux_${ARCH} && \
    echo "${!HELM_SHA256}  /usr/tmp/${HELM_TARBALL}" | sha256sum -c - && \
    tar xvzf /usr/tmp/${HELM_TARBALL} --strip-components=1 -C /usr/tmp/ && \
    mv /usr/tmp/helm /usr/bin/helm

# -- for make rules
## install docker client
RUN apt-get update -qq && apt-get install -y --no-install-recommends \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg \
    rsync \
    && rm -rf /var/lib/apt/lists/*; \
    \
    curl -fsSL https://download.docker.com/linux/debian/gpg | apt-key add - >/dev/null; \
    echo "deb [arch=$(dpkg --print-architecture)] https://download.docker.com/linux/debian buster stable" > /etc/apt/sources.list.d/docker.list; \
    \
    apt-get update -qq && apt-get install -y --no-install-recommends \
    docker-ce=5:20.10.* \
    && rm -rf /var/lib/apt/lists/*
## install golangci
COPY --from=golangci/golangci-lint:v2.12.2-alpine@sha256:91b27804074a0bacea298707f016911e60cf0cdbc6c7bf5ccacb5f0606d18d60 /usr/bin/golangci-lint /usr/local/bin/golangci-lint

## install controller-gen
RUN GO111MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.1
## install ginkgo
RUN GO111MODULE=on go install github.com/onsi/ginkgo/v2/ginkgo@v2.29.0

# install openapi-gen
RUN GO111MODULE=on go install k8s.io/code-generator/cmd/openapi-gen@v0.29.13

# install kind
RUN curl -Lo /usr/bin/kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${ARCH} && \
    KIND_SHA256="KIND_SHA256_Linux_${ARCH}" && \
    echo "${!KIND_SHA256}  /usr/bin/kind" | sha256sum -c - && \
    chmod +x /usr/bin/kind

# install codecov
RUN curl -Lo /usr/bin/codecov https://uploader.codecov.io/${CODECOV_VERSION}/linux/codecov && \
    echo "${CODECOV_SHA256_Linux}  /usr/bin/codecov" | sha256sum -c - && \
    chmod +x /usr/bin/codecov


ENV HOME=/go/src/github.com/harvester/harvester


# ---- base ----
FROM builder AS base
WORKDIR /go/src/github.com/harvester/harvester

# to exclude some files, add them in .dockerignore
COPY . .


# ---- build ----
FROM base AS build
ARG MK_REPO_ID

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/build

FROM scratch AS build-output
COPY --from=build /go/src/github.com/harvester/harvester/bin/ /bin/


# ---- prepare-addons ----
FROM builder AS prepare-addons
# Re-declare them here so this specific stage inherits global ARGs at the top of the Dockerfile, and can be used in this stage.
ARG ADDONS_REPO
ARG ADDONS_BRANCH

ARG ADDONS_PREPARE_DIR
ENV ADDONS_PREPARE_DIR=${ADDONS_PREPARE_DIR}

# re-pull when remote sha changed
ARG REMOTE_SHA=unknown

RUN mkdir -p "${ADDONS_PREPARE_DIR}"
# clone addons repo
RUN git clone --branch "${ADDONS_BRANCH}" --single-branch --depth 1 \
    "${ADDONS_REPO}" "${ADDONS_PREPARE_DIR}/addons" && \
    rm -rf "${ADDONS_PREPARE_DIR}/addons/.git"
    
# generate addon manifests
RUN mkdir -p "${ADDONS_PREPARE_DIR}/addons-manifests" && \
    cd "${ADDONS_PREPARE_DIR}/addons" && \
    go run . -generateAddons -path "${ADDONS_PREPARE_DIR}/addons-manifests"

# generate addon templates (for rancherd)
RUN mkdir -p "${ADDONS_PREPARE_DIR}/addons-templates" && \
    cd "${ADDONS_PREPARE_DIR}/addons" && \
    go run . -generateTemplates -path "${ADDONS_PREPARE_DIR}/addons-templates"


# ---- validate ----
FROM base AS validate
ARG MK_REPO_ID

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/validate


# ---- validate-ci ----
FROM base AS validate-ci
ARG MK_REPO_ID

# Init a new git repo for copied files inside the container, the test script checks if files are
# modified after running go generate
RUN git config --global user.email "ci@example.com" && \
    git config --global user.name "ci" && \
    git init 2>/dev/null && git add . && git commit -q -m "commit for validate-ci"

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/validate-ci


# ---- test ----
FROM base AS test
ARG MK_REPO_ID

ARG ADDONS_PREPARE_DIR
ENV ADDONS_PREPARE_DIR=${ADDONS_PREPARE_DIR}

COPY --from=prepare-addons "${ADDONS_PREPARE_DIR}"/addons-templates/rancherd-22-addons.yaml \
     /go/src/github.com/harvester/harvester/pkg/installer/config/templates/rancherd-22-addons.yaml
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    --mount=type=secret,id=codecov_token_${MK_REPO_ID} \
    CODECOV_TOKEN=$(cat /run/secrets/codecov_token_${MK_REPO_ID} 2>/dev/null || true) \
    ./scripts/test


# ---- test-integration ----
FROM base AS test-integration


# ---- generate-manifest ----
FROM base AS generate-manifest
RUN ./scripts/generate-manifest

FROM scratch AS generate-manifest-output
COPY --from=generate-manifest /go/src/github.com/harvester/harvester/deploy/charts/harvester-crd/templates/ /templates/


# ---- generate-openapi ----
FROM base AS generate-openapi
ARG MK_REPO_ID

RUN git config --global user.email "ci@example.com" && \
    git config --global user.name "ci" && \
    git init 2>/dev/null && git add . && git commit -q -m "commit for validate-ci"

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/generate-openapi

FROM scratch AS generate-openapi-output
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/api/openapi-spec/swagger.json /api/openapi-spec/swagger.json
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/scripts/known-api-rule-violations.txt /scripts/known-api-rule-violations.txt


# ---- build-installer ----
FROM base AS build-installer
ARG MK_REPO_ID
# 1. Re-declare them here so this specific stage inherits global ARGs at the top of the Dockerfile, and can be used in this stage.
ARG ADDONS_REPO
ARG ADDONS_BRANCH

ARG ADDONS_PREPARE_DIR
ENV ADDONS_PREPARE_DIR=${ADDONS_PREPARE_DIR}

# 2. Convert the ARGs into ENVs so that the build-installer script can access them natively
ENV ADDONS_REPO=${ADDONS_REPO}
ENV ADDONS_BRANCH=${ADDONS_BRANCH}

COPY --from=prepare-addons "${ADDONS_PREPARE_DIR}/addons/" /go/src/github.com/harvester/addons/

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/build-installer

FROM scratch AS build-installer-output
COPY --from=build-installer /go/src/github.com/harvester/harvester/bin/harvester-installer /bin/


# ---- bundle-builder ----
FROM builder AS bundle-builder

WORKDIR /go/src/github.com/harvester/harvester

COPY scripts/images/*.yaml scripts/images/
COPY scripts/lib/ scripts/lib/


# ---- prepare-addons-charts ----
FROM bundle-builder AS prepare-addons-charts

ARG ADDONS_PREPARE_DIR
ENV ADDONS_PREPARE_DIR=${ADDONS_PREPARE_DIR}

COPY --from=prepare-addons "${ADDONS_PREPARE_DIR}/addons/" /go/src/github.com/harvester/addons/
COPY --from=prepare-addons "${ADDONS_PREPARE_DIR}/addons-templates/" /go/src/github.com/harvester/addons-templates/

COPY scripts/images/rancher-images.txt scripts/images/rancher-images.txt
COPY scripts/prepare-addons-charts scripts/prepare-addons-charts
RUN bash scripts/prepare-addons-charts


# ---- prepare-harvester-charts ----
FROM bundle-builder AS prepare-harvester-charts

COPY deploy/ deploy/
COPY scripts/prepare-harvester-charts scripts/prepare-harvester-charts
COPY scripts/patch-harvester scripts/patch-harvester
COPY scripts/version scripts/.version_env scripts/
RUN bash scripts/prepare-harvester-charts


# ---- check-images ----
FROM bundle-builder AS check-images

COPY scripts/check-images scripts/check-images
COPY scripts/version-rancher scripts/version-rancher
COPY scripts/images/rancher-images.txt scripts/images/rancherd-bootstrap-images.txt scripts/images/
RUN bash scripts/check-images


# ---- build-iso ----
FROM builder AS build-iso

ARG ADDONS_PREPARE_DIR
ENV ADDONS_PREPARE_DIR=${ADDONS_PREPARE_DIR}

WORKDIR /go/src/github.com/harvester/harvester

COPY --from=build-installer /go/src/github.com/harvester/harvester/bin/harvester-installer package/harvester-os/files/usr/bin/
COPY --from=prepare-addons "${ADDONS_PREPARE_DIR}/addons/" /go/src/github.com/harvester/addons/
COPY --from=prepare-harvester-charts /go/src/github.com/harvester/harvester/deploy/charts/ /go/src/github.com/harvester/harvester/deploy/charts/
COPY --from=prepare-harvester-charts /dist/chart-tarballs/* /go/src/github.com/harvester/harvester/package/harvester-repo/charts/
COPY --from=prepare-addons-charts /dist/charts/*.tgz /go/src/github.com/harvester/harvester/package/harvester-repo/charts/

COPY scripts/ scripts/
COPY package/upgrade-matrix.yaml package/upgrade-matrix.yaml
COPY package/harvester-os/ package/harvester-os/
COPY package/harvester-repo/ package/harvester-repo/
