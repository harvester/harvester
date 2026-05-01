# syntax=docker/dockerfile:1
# check=skip=InvalidDefaultArgInFrom
ARG MK_BUILDER_IMAGE
FROM ${MK_BUILDER_IMAGE} AS builder

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

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/validate-ci


# ---- test ----
FROM base AS test
ARG MK_REPO_ID

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    --mount=type=secret,id=codecov_token_${MK_REPO_ID},env=CODECOV_TOKEN \
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

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/generate-openapi

FROM scratch AS generate-openapi-output
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/api/openapi-spec/swagger.json /api/openapi-spec/swagger.json
COPY --from=generate-openapi /go/src/github.com/harvester/harvester/scripts/known-api-rule-violations.txt /scripts/known-api-rule-violations.txt


# ---- prepare-addons ----
FROM builder AS prepare-addons
ARG ADDONS_BRANCH=main

# re-pull when remote sha changed
ARG REMOTE_SHA=unknown

RUN mkdir -p /dist/prepare-addons
# clone addons repo
RUN git clone --branch ${ADDONS_BRANCH} --single-branch --depth 1 \
    https://github.com/harvester/addons.git /dist/prepare-addons/addons && \
    rm -rf /dist/prepare-addons/addons/.git
    
# generate addon manifests
RUN mkdir -p /dist/prepare-addons/addons-manifests && \ 
    cd /dist/prepare-addons/addons && \
    go run . -generateAddons -path /dist/prepare-addons/addons-manifests


# ---- build-iso ----
FROM base AS build-iso
