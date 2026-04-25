ROOT              := $(realpath $(dir $(realpath $(firstword $(MAKEFILE_LIST)))))
comma             := ,

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

# User might have several repos in a host. Distinguish each by using the abs path of the repo
MK_REPO_ID               := $(shell echo -n "$(ROOT)$$(cat /etc/machine-id 2>/dev/null)" | sha256sum | cut -c1-8)
MK_BUILDER_IMAGE         := harvester-builder:$(MK_REPO_ID)
MK_ADDONS_IMAGE          := harvester-addons:$(MK_REPO_ID)
MK_ISO_BUILDER_IMAGE     := harvester-iso-builder:$(MK_REPO_ID)
MK_DOCKER_PROGRESS       ?= auto

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

export MK_DOCKER_PROGRESS MK_BUILDER_IMAGE MK_REPO_ID MK_ADDONS_IMAGE MK_ISO_BUILDER_IMAGE
export HARVESTER_UI_VERSION HARVESTER_UI_PLUGIN_BUNDLED_VERSION
export HARVESTER_INSTALLER_REPO HARVESTER_INSTALLER_REF RKE2_IMAGE_REPO USE_LOCAL_IMAGES REPO PUSH

HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

DOCKER_BUILD = docker build \
	--progress=$(MK_DOCKER_PROGRESS) \
	--build-arg MK_BUILDER_IMAGE \
	--build-arg MK_REPO_ID \
	-f $(ROOT)/Dockerfile $(ROOT)

.PHONY: builder-image build validate validate-ci test test-integration build-iso \
	package-all package package-harvester-webhook package-harvester-upgrade \
	generate-manifest generate-openapi prepare-addons ci arm clean clean-all default


# ---- Directories ----
$(ROOT)/bin:
	@mkdir -p $@


# ---- Builder image ----
builder-image:
	$(BANNER)
	docker build \
	    --build-arg CONTAINER_WORKDIR=/go/src/github.com/harvester/harvester \
	    --build-arg HOST_ARCH=$(HOST_ARCH) \
	    -f $(ROOT)/Dockerfile.builder \
	    -t $(MK_BUILDER_IMAGE) \
	    $(ROOT)


# ---- Compile harvester binaries ----
build: builder-image | $(ROOT)/bin
	$(BANNER)
	$(DOCKER_BUILD) --target build-output --output type=local,dest=.


# ---- Validate ----
validate: builder-image
	$(BANNER)
	$(DOCKER_BUILD) --target validate


# ---- Validate CI (dirty check after go generate + go mod tidy) ----
validate-ci: builder-image
	$(BANNER)
	$(DOCKER_BUILD) --target validate-ci


# ---- Test ----
test: builder-image
	$(BANNER)
	$(DOCKER_BUILD) $(if $(CODECOV_TOKEN),--secret id=codecov_token$(comma)env=CODECOV_TOKEN --no-cache-filter=test) --target test


# ---- Test integration ----
test-integration: builder-image
	$(BANNER)
	$(DOCKER_BUILD) --target test-integration -t harvester-test-integration:$(MK_REPO_ID)
	docker run --rm --privileged --network host \
	    -v /var/run/docker.sock:/var/run/docker.sock \
		-v harvester-test-integration-go-cache-${MK_REPO_ID}:/go/src/github.com/harvester/harvester/.cache/go-build \
	    harvester-test-integration:$(MK_REPO_ID) \
	    ./scripts/test-integration


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
generate-manifest: builder-image
	$(BANNER)
	$(DOCKER_BUILD) --target generate-manifest-output --output type=local,dest=$(ROOT)/deploy/charts/harvester-crd/


# ---- Generate OpenAPI/Swagger spec ----
generate-openapi: builder-image
	$(BANNER)
	$(DOCKER_BUILD) --target generate-openapi-output --output type=local,dest=$(ROOT)


# ---- Cache addons repo and generate addons manifests ---
prepare-addons: builder-image
	$(BANNER)
	$(ROOT)/scripts/prepare-addons
	# $(DOCKER_BUILD) --target prepare-addons-output --output type=local,dest=.


# ---- Build ISO ----
build-iso: builder-image
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
	@docker rmi -f $(MK_BUILDER_IMAGE) $(MK_ADDONS_IMAGE) || true
	@docker rmi -f harvester-iso-builder:$(MK_REPO_ID) harvester-test-integration:$(MK_REPO_ID) || true

.DEFAULT_GOAL := default

default: build test package-all

arm: build package-all

ci: validate validate-ci build test package-harvester-webhook package-harvester-upgrade \
	test-integration package
