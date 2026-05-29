ROOT              := $(realpath $(dir $(realpath $(firstword $(MAKEFILE_LIST)))))
comma             := ,

# some systems requires opt-in for buildx
DOCKER_BUILDKIT   := 1
export DOCKER_BUILDKIT

ifdef CI
  BOLD  :=
  CYAN  :=
  RESET :=
else
  BOLD  := \033[1m
  CYAN  := \033[36m
  RESET := \033[0m
endif

BANNER = @printf "$(BOLD)$(CYAN)[target: $@]$(RESET)\n"

# Allocate a TTY in dev (for ctrl+c) but not in CI
MK_DOCKER_RUN_OPTS_TTY := $(if $(CI),,-it)
export MK_DOCKER_RUN_OPTS_TTY

# Safely detect a unique system identifier into a variable
MK_SYSTEM_ID := $(strip $(shell \
    if [ -s /etc/machine-id ]; then \
        cat /etc/machine-id 2>/dev/null; \
    elif command -v hostname >/dev/null 2>&1; then \
        hostname 2>/dev/null; \
    else \
        echo -n "unknown"; \
    fi))

# User might have several repos in a host. Distinguish each by using the abs path of the repo
MK_REPO_ID                := $(shell printf '%s' "$(ROOT)$(MK_SYSTEM_ID)" | sha256sum | cut -c1-8)
MK_ADDONS_IMAGE           := harvester-addons:$(MK_REPO_ID)
MK_ISO_BUILDER_IMAGE      := harvester-iso-builder:$(MK_REPO_ID)
MK_TEST_INTEGRATION_IMAGE := harvester-test-integration:$(MK_REPO_ID)
MK_DOCKER_PROGRESS        ?= plain

# Legacy dapper env variables
CODECOV_TOKEN             ?=
HARVESTER_ADDONS_VERSION  ?= main
HARVESTER_INSTALLER_REPO  ?=
HARVESTER_INSTALLER_REF   ?=
HARVESTER_UI_VERSION      ?=
HARVESTER_UI_PLUGIN_BUNDLED_VERSION ?=
RKE2_IMAGE_REPO           ?=
USE_LOCAL_IMAGES          ?=
REPO                      ?=
PUSH                      ?=
DRONE_BRANCH              ?=
DRONE_TAG                 ?=

export MK_DOCKER_PROGRESS MK_REPO_ID MK_ADDONS_IMAGE MK_ISO_BUILDER_IMAGE
export HARVESTER_UI_VERSION HARVESTER_UI_PLUGIN_BUNDLED_VERSION
export HARVESTER_INSTALLER_REPO HARVESTER_INSTALLER_REF RKE2_IMAGE_REPO USE_LOCAL_IMAGES REPO PUSH DRONE_BRANCH DRONE_TAG
export CODECOV_TOKEN HARVESTER_ADDONS_VERSION

MK_HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
export MK_HOST_ARCH

DOCKER_BUILD = docker build \
	--progress=$(MK_DOCKER_PROGRESS) \
	--build-arg MK_REPO_ID \
	--build-arg MK_HOST_ARCH \
	-f $(ROOT)/Dockerfile $(ROOT)

.PHONY: build validate validate-ci test test-integration build-iso \
	package-all package package-harvester-webhook package-harvester-upgrade \
	generate-manifest generate-openapi prepare-addons ci arm clean clean-all default \
	gen-version-env gen-version-env-debug integration-check


# ---- Directories ----
$(ROOT)/bin:
	@mkdir -p $@


# ---- Pre-generate version env for container builds (no .git needed inside Docker) ----
# Also handles git worktree checkouts where .git is a pointer file to an external directory.
gen-version-env:
	$(BANNER)
	@bash $(ROOT)/scripts/version > /dev/null


# ---- Generate and show the version env for debugging ----
gen-version-env-debug:
	$(BANNER)
	bash $(ROOT)/scripts/version debug


# ---- Compile harvester binaries ----
build: gen-version-env | $(ROOT)/bin
	$(BANNER)
	$(DOCKER_BUILD) --target build-output --output type=local,dest=.


# ---- Validate ----
validate: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target validate


# ---- Validate CI (dirty check after go generate + go mod tidy) ----
validate-ci: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target validate-ci


# ---- Test ----
test: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) $(if $(CODECOV_TOKEN),--secret id=codecov_token_$(MK_REPO_ID)$(comma)env=CODECOV_TOKEN --no-cache-filter=test) --target test


# ---- Test integration ----
test-integration: gen-version-env package-harvester-webhook
	$(BANNER)
	$(DOCKER_BUILD) --target test-integration -t $(MK_TEST_INTEGRATION_IMAGE)
	docker run $(MK_DOCKER_RUN_OPTS_TTY) --rm --privileged --network host \
	    -v /var/run/docker.sock:/var/run/docker.sock \
	    -v harvester-test-integration-go-cache-${MK_REPO_ID}:/go/src/github.com/harvester/harvester/.cache/go-build \
	    $(MK_TEST_INTEGRATION_IMAGE) \
	    ./scripts/test-integration


# ---- Integration check ----
integration-check: build
	$(BANNER)
	$(DOCKER_BUILD) --target test-integration -t $(MK_TEST_INTEGRATION_IMAGE)
	docker run $(MK_DOCKER_RUN_OPTS_TTY) --rm \
	    -v harvester-integration-check-go-cache-${MK_REPO_ID}:/go/src/github.com/harvester/harvester/.cache/go-build \
	    $(MK_TEST_INTEGRATION_IMAGE) \
	    ./scripts/integration-check


# ---- Package harvester image ----
package: build
	$(BANNER)
	$(ROOT)/scripts/package


# ---- Package harvester-webhook image ----
package-harvester-webhook: build
	$(BANNER)
	$(ROOT)/scripts/package-webhook


# ---- Package harvester-upgrade image ----
package-harvester-upgrade: build prepare-addons
	$(BANNER)
	$(ROOT)/scripts/package-upgrade


# ---- Package all images ----
package-all: package package-harvester-webhook package-harvester-upgrade


# ---- Generate CRD manifests ----
generate-manifest: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target generate-manifest-output --output type=local,dest=$(ROOT)/deploy/charts/harvester-crd/


# ---- Generate OpenAPI/Swagger spec ----
generate-openapi: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target generate-openapi-output --output type=local,dest=$(ROOT)


# ---- Cache addons repo and generate addons manifests ---
prepare-addons:
	$(BANNER)
	$(ROOT)/scripts/prepare-addons


# ---- Build ISO ----
build-iso: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target build-iso -t $(MK_ISO_BUILDER_IMAGE)
	$(ROOT)/scripts/mk-build-iso


# ---- Clean ----
clean:
	$(BANNER)
	@rm -rf $(ROOT)/bin
	@rm -f $(ROOT)/package/harvester $(ROOT)/package/harvester-webhook
	@rm -f $(ROOT)/package/upgrade/upgrade-helper
	@rm -rf $(ROOT)/package/upgrade/addons
	@rm -rf $(ROOT)/dist/prepare-addons
	@rm -rf $(ROOT)/dist/artifacts $(ROOT)/dist/harvester-cluster-repo

clean-all: clean
	$(BANNER)
	@docker rmi -f $(MK_ADDONS_IMAGE) || true
	@docker rmi -f $(MK_ISO_BUILDER_IMAGE) $(MK_TEST_INTEGRATION_IMAGE) || true

.DEFAULT_GOAL := default

default: build test package-all

arm: build package-all

ci: validate validate-ci build test package-harvester-webhook package-harvester-upgrade \
	package
