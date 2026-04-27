# Build System

Harvester's build system uses **GNU Make + Docker BuildKit**. Every build target is a stage in a single `Dockerfile` at the repo root, driven by `make`. There is no Dapper dependency.

## Quick start

```bash
make                  # build binaries, run tests, package all images (default)
make build            # compile harvester, harvester-webhook, upgrade-helper
make test             # run unit tests
make package-all      # build all container images
make validate         # run linters
make validate-ci      # dirty-check (go generate + go mod tidy)
make build-iso        # build the Harvester ISO
make prepare-addons   # fetch and cache addons repo + generate manifests
make generate-manifest  # regenerate CRD manifest templates
make generate-openapi   # regenerate OpenAPI/Swagger spec
```

Pass `CODECOV_TOKEN=<token>` to upload coverage after `make test`.

---

## Architecture

```
Makefile                      ← orchestrates everything
Dockerfile                    ← one stage per build target
Dockerfile.builder            ← builder base image definition
scripts/<name>                ← scripts that run INSIDE the container. Except package scripts.
scripts/mk-<name>             ← host-side helpers for DinD targets
```

### Makefile

Two key variables drive all docker builds:

```makefile
# builder base image, tagged per-repo to isolate caches
MK_BUILDER_IMAGE := harvester-builder:$(MK_REPO_ID)

# shared docker build flags
DOCKER_BUILD = docker build \
    --progress=$(MK_DOCKER_PROGRESS) \
    --build-arg MK_BUILDER_IMAGE \
    --build-arg MK_REPO_ID \
    -f $(ROOT)/Dockerfile $(ROOT)
```

`MK_REPO_ID` is an 8-character hash of the repo path + machine ID, so multiple checkouts on the same host get separate builder images and BuildKit caches.

Colors in banner output are suppressed when the `CI` environment variable is set:

```makefile
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
```

### Dockerfile

One file, multiple stages. The stage hierarchy:

```
Dockerfile.builder  →  builder image (MK_BUILDER_IMAGE)
                            │
                         builder  (ARG alias stage)
                            │
                           base   (COPY . . — includes .git)
                          /   \
                       build   validate  validate-ci  test  ...
```

`base` copies the entire repo including `.git` so every stage has a live git history — needed for `version`, `validate-ci` (dirty check), and `generate-openapi` (restores vendor files via `git checkout`).

```
Dockerfile.builder  →  builder image (MK_BUILDER_IMAGE)
                             │
                        prepare-addons
```
`prepare-addons` inherits from `builder` instead of `base` because it clones the harvester add-ons into `/dist/` rather than working with the harvester source tree.

### BuildKit cache mounts

Go stages use two BuildKit cache mounts scoped by `MK_REPO_ID`:

```dockerfile
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/<name>
```

These caches persist across builds and are isolated per repo checkout. `generate-manifest` skips them (it runs `controller-gen`, not the Go compiler).

---

## Stage types

### Standard stage — runs a script, no output

```dockerfile
# ---- validate ----
FROM base AS validate
ARG MK_REPO_ID

RUN --mount=type=cache,...
    ./scripts/validate
```

```makefile
validate: builder-image
    $(BANNER)
    $(DOCKER_BUILD) --target validate
```

### Extraction stage — produces files on the host

Add a `FROM scratch AS <name>-output` stage and use `--output type=local`:

```dockerfile
FROM base AS build
ARG MK_REPO_ID
RUN --mount=type=cache,... ./scripts/build

FROM scratch AS build-output
COPY --from=build /go/src/github.com/harvester/harvester/bin/ /bin/
```

```makefile
build: builder-image | $(ROOT)/bin
    $(BANNER)
    $(DOCKER_BUILD) --target build-output --output type=local,dest=.
```

The `--output type=local,dest=<path>` flag extracts the scratch stage contents to the host. No `docker cp` needed.

### DinD stage — needs Docker socket at runtime

Targets like `test-integration` and `build-iso` need the Docker socket and cannot use `docker build` RUN steps for the actual work. The Dockerfile stage is left **empty** (just establishes the image), and the real execution happens via `docker run`:

```dockerfile
# ---- test-integration ----
FROM base AS test-integration
```

```makefile
test-integration: builder-image
    $(BANNER)
    $(DOCKER_BUILD) --target test-integration -t harvester-test-integration:$(MK_REPO_ID)
    docker run --rm --privileged --network host \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v harvester-test-integration-go-cache-$(MK_REPO_ID):/go/src/github.com/harvester/harvester/.cache/go-build \
        harvester-test-integration:$(MK_REPO_ID) \
        ./scripts/test-integration
```

For Go-based DinD targets, pass a named volume for the Go build cache so repeated runs avoid recompiling.

#### DinD with conditional env vars — `scripts/mk-<target>`

When a DinD target needs optional env var passthrough, the `docker run` command is extracted to `scripts/mk-<target>` to avoid Makefile `$(if ...)` with commas (which conflict with Make's argument separator). Use bash `${VAR:+--env VAR="$VAR"}` for optional vars:

```bash
#!/bin/bash
# scripts/mk-build-iso
set -e

TOP_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
MK_REPO_ID="${MK_REPO_ID:?MK_REPO_ID is required}"
CONTAINER_NAME="harvester-iso-builder-run-${MK_REPO_ID}"

docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
cleanup() { docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run --privileged \
    --name "${CONTAINER_NAME}" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    ${HARVESTER_INSTALLER_REF:+--env HARVESTER_INSTALLER_REF="$HARVESTER_INSTALLER_REF"} \
    "${MK_ISO_BUILDER_IMAGE}" \
    ./scripts/build-iso

# extract artifacts from the named container before cleanup
docker cp "${CONTAINER_NAME}:/go/src/github.com/harvester/harvester/dist/artifacts/." "${TOP_DIR}/dist/artifacts/"
docker cp "${CONTAINER_NAME}:/go/src/github.com/harvester/harvester/dist/harvester-cluster-repo" "${TOP_DIR}/dist/"

trap - EXIT
docker rm "${CONTAINER_NAME}" >/dev/null
```

The Makefile calls it simply:

```makefile
build-iso: builder-image
    $(BANNER)
    $(DOCKER_BUILD) --target build-iso -t $(MK_ISO_BUILDER_IMAGE)
    $(ROOT)/scripts/mk-build-iso
```

---

## Variables reference

| Variable | Default | Purpose |
|---|---|---|
| `MK_REPO_ID` | _(computed)_ | 8-char hash isolating caches per checkout |
| `MK_BUILDER_IMAGE` | `harvester-builder:$(MK_REPO_ID)` | Builder image name |
| `MK_ADDONS_IMAGE` | `harvester-addons:$(MK_REPO_ID)` | Addons cache image |
| `MK_ISO_BUILDER_IMAGE` | `harvester-iso-builder:$(MK_REPO_ID)` | ISO builder image |
| `MK_DOCKER_PROGRESS` | `auto` | BuildKit progress display (`auto`, `plain`) |
| `HARVESTER_ADDONS_VERSION` | `main` | Branch to clone for addons |
| `HARVESTER_INSTALLER_REPO` | _(empty)_ | Override installer git URL for `build-iso` |
| `HARVESTER_INSTALLER_REF` | _(empty)_ | Override installer branch/tag for `build-iso` |
| `RKE2_IMAGE_REPO` | _(empty)_ | Override RKE2 image repo |
| `USE_LOCAL_IMAGES` | _(empty)_ | Use locally cached images in ISO build |
| `REPO` | _(empty)_ | Docker registry prefix for packaged images |
| `PUSH` | _(empty)_ | Set non-empty to push images after packaging |
| `CODECOV_TOKEN` | _(empty)_ | Upload coverage to Codecov after `make test` |

All variables can be overridden on the command line: `make build-iso HARVESTER_INSTALLER_REF=v1.5.0` or from environment variables.

---

## Migrating a target from Dapper

### Step 1 — Identify the script type

Scripts fall into two categories:

**Orchestration scripts** (delete, do not convert): these just `cd $(dirname $0)` and call sibling scripts sequentially (`./build`, `./test`, etc.). They were Dapper's entry points and are fully replaced by Makefile targets. Examples: `scripts/ci`, `scripts/arm`, `scripts/default`, `scripts/entry`.

**Build scripts** (convert): scripts that actually compile, test, generate, or build images. Examples: `scripts/build`, `scripts/validate`, `scripts/build-iso`.

### Step 2 — Add a Dockerfile stage

Choose the stage type based on what the script does:

| Script does | Stage type |
|---|---|
| Compile / validate / generate (no Docker) | Standard: `FROM base AS <name>` + `RUN ./scripts/<name>` |
| Produces files to extract to host | Add `FROM scratch AS <name>-output` + `--output type=local` |
| Needs Docker socket at runtime | DinD: empty `FROM base AS <name>`, run via `docker run` |

Add the stage to `Dockerfile` in the appropriate group. Go stages need the cache mounts:

```dockerfile
# ---- <target-name> ----
FROM base AS <target-name>
ARG MK_REPO_ID

RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester/.cache/go-build,id=harvester-go-build-${MK_REPO_ID} \
    ./scripts/<name>
```

### Step 3 — Add a Makefile target

```makefile
# ---- <description> ----
<target-name>: builder-image
    $(BANNER)
    $(DOCKER_BUILD) --target <target-name>
```

Add the target name to `.PHONY`. For extraction targets, append `--output type=local,dest=$(ROOT)/<path>`. For DinD targets, build the image first then `docker run` (or delegate to `scripts/mk-<target-name>`).

### Step 4 — Update `clean` / `clean-all`

Add `@rm -rf` lines to `clean` for any files extracted to the host. Add `@docker rmi -f` lines to `clean-all` for any images tagged by the target.

### Step 5 — Validate

- Every `ARG` used inside a stage body is re-declared in that stage (ARGs do not carry between stages)
- `--output type=local` destination path matches the `COPY` paths in the scratch stage
- DinD targets use `--privileged` and mount `/var/run/docker.sock`
- `.PHONY` includes the new target
