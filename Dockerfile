FROM quay.io/costoolkit/releases-teal:grub2-live-0.0.4-2 AS grub2-mbr
FROM quay.io/costoolkit/releases-teal:grub2-efi-image-live-0.0.4-2 AS grub2-efi

FROM golang:1.25.7-bookworm AS builder

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
COPY --from=golangci/golangci-lint:v2.8.0-alpine@sha256:1194f3bfcbaeeb92d8d159fdfbe2a79d18ec0a222d9d984b1438906bca416b51 /usr/bin/golangci-lint /usr/local/bin/golangci-lint

## install controller-gen
RUN GO111MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.1
## install ginkgo
RUN GO111MODULE=on go install github.com/onsi/ginkgo/v2/ginkgo@v2.23.4

# install openapi-gen
RUN  GO111MODULE=on go install k8s.io/code-generator/cmd/openapi-gen@v0.29.13

# install kind
RUN curl -Lo /usr/bin/kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${ARCH} && \
    KIND_SHA256="KIND_SHA256_Linux_${ARCH}" && \
    echo "${!KIND_SHA256}  /usr/bin/kind" | sha256sum -c - && \
    chmod +x /usr/bin/kind

# install codecov
RUN curl -Lo /usr/bin/codecov https://uploader.codecov.io/${CODECOV_VERSION}/linux/codecov && \
    echo "${CODECOV_SHA256_Linux}  /usr/bin/codecov" | sha256sum -c - && \
    chmod +x /usr/bin/codecov

# copy bootloaders
RUN mkdir /grub2-mbr
COPY --from=grub2-mbr / /grub2-mbr
RUN mkdir /grub2-efi
COPY --from=grub2-efi / /grub2-efi

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

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    --mount=type=secret,id=codecov_token_${MK_REPO_ID} \
    CODECOV_TOKEN=$(cat /run/secrets/codecov_token_${MK_REPO_ID} 2>/dev/null || true) \
    ./scripts/test


# ---- test-integration ----
FROM base AS test-integration


# ---- git-setup-stage ----
# Handles caching and dynamically configures the Git environment for both local and CI builds.
# Worktree-aware: Mirrors both worktree-specific configurations and shared common object directories.
FROM base AS git-setup-stage
ARG MK_REPO_ID
RUN --mount=type=bind,from=git-dir,target=/tmp/host-git \
--mount=type=bind,from=git-common,target=/tmp/host-git-common \
--mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
--mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
if [ -d /tmp/host-git/objects ]; then \
    echo "✅ Authentic Git repository detected from host. Mirroring configurations..."; \
    mkdir -p /go/src/github.com/harvester/harvester/.git; \
    cp -r /tmp/host-git/* /go/src/github.com/harvester/harvester/.git/ 2>/dev/null || true; \
elif [ -d /tmp/host-git-common/objects ]; then \
    echo "✅ Git worktree workspace detected. Mirroring configs and linking shared core objects..."; \
    mkdir -p /go/src/github.com/harvester/harvester/.git; \
    cp -r /tmp/host-git/* /go/src/github.com/harvester/harvester/.git/ 2>/dev/null || true; \
    rm -rf /go/src/github.com/harvester/harvester/.git/objects; \
    ln -s /tmp/host-git-common/objects /go/src/github.com/harvester/harvester/.git/objects; \
else \
    echo "⚠️ No host Git data found (Fresh CI Environment). Initializing self-healing fake Git layer..."; \
    git config --global user.email "ci@example.com" && \
    git config --global user.name "ci" && \
    git init 2>/dev/null && \
    git add . && \
    git commit -q -m "automated commit for validate-ci"; \
fi

# ---- generate-stage (`go generate`) ----
# Runs full go generate (which automatically triggers manifests and openapi via go:generate directives)
FROM git-setup-stage AS generate-stage
RUN ./scripts/generate


# ---- generate-manifest ----
# Standalone target branch: Runs only manifest generation from a clean git environment
FROM git-setup-stage AS generate-manifest
RUN ./scripts/generate-manifest


# ---- generate-openapi ----
# Standalone target branch: Runs only openapi generation from a clean git environment
FROM git-setup-stage AS generate-openapi
RUN ./scripts/generate-openapi


# ---- generate-output ----
# Since generate-stage already ran EVERYTHING via go generate, we pull all assets straight from it!
FROM scratch AS generate-output
COPY --from=generate-stage /go/src/github.com/harvester/harvester/pkg/apis/ /pkg/apis/
COPY --from=generate-stage /go/src/github.com/harvester/harvester/pkg/generated/ /pkg/generated/
COPY --from=generate-stage /go/src/github.com/harvester/harvester/deploy/charts/harvester-crd/templates/ /deploy/charts/harvester-crd/templates/
COPY --from=generate-stage /go/src/github.com/harvester/harvester/api/openapi-spec/swagger.json /api/openapi-spec/swagger.json
COPY --from=generate-stage /go/src/github.com/harvester/harvester/scripts/known-api-rule-violations.txt /scripts/known-api-rule-violations.txt


# ---- generate-manifest-output ----
FROM scratch AS generate-manifest-output
COPY --from=generate-manifest /go/src/github.com/harvester/harvester/deploy/charts/harvester-crd/templates/ /deploy/charts/harvester-crd/templates/


# ---- generate-openapi-output ----
FROM scratch AS generate-openapi-output
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/api/openapi-spec/swagger.json /api/openapi-spec/swagger.json
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/scripts/known-api-rule-violations.txt /scripts/known-api-rule-violations.txt


# ---- prepare-addons ----
FROM builder AS prepare-addons
ARG ADDONS_BRANCH

# re-pull when remote sha changed
ARG REMOTE_SHA

RUN mkdir -p /dist/prepare-addons
# clone addons repo
RUN echo "REMOTE_SHA=${REMOTE_SHA}" && \
    git clone --branch ${ADDONS_BRANCH} --single-branch --depth 1 \
    https://github.com/harvester/addons.git /dist/prepare-addons/addons
    
# generate addon manifests
RUN mkdir -p /dist/prepare-addons/addons-manifests && \
    cd /dist/prepare-addons/addons && \
    go run . -generateAddons -path /dist/prepare-addons/addons-manifests


# ---- build-iso ----
FROM base AS build-iso
