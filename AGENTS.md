# AGENTS.md

Harvester is an open-source bare-metal Hyperconverged Infrastructure (HCI) platform built on Kubernetes. It integrates virtual machine management (KubeVirt), distributed block storage (Longhorn), and cluster management (Rancher) into a cohesive platform. This repository contains the **Harvester API server** — a Go binary that implements Kubernetes controllers, admission webhooks, and REST API handlers for all Harvester resources.

## Repository Structure

```
./                                     # Project root
├── main.go                            # Entry point; CLI setup; go:generate directives
├── Makefile                           # All build targets
├── Dockerfile                         # Multi-stage: builder → validate → test → package → ISO
├── go.mod / go.sum                    # Go module (many replace directives for Rancher/Wrangler forks)
├── vendor/                            # Vendored dependencies (committed; always run go mod vendor)
├── api/openapi-spec/                  # Generated OpenAPI/Swagger spec
├── cmd/                               # Additional binaries (upgradehelper, webhook server)
├── pkg/
│   ├── apis/harvesterhci.io/v1beta1/  # CRD Go types (hand-written; generated files prefixed zz_)
│   ├── generated/                     # Code-generated clients, informers, listers (DO NOT EDIT)
│   ├── config/                        # Shared Management context — all controller/client factories
│   ├── controller/
│   │   ├── master/                    # Master-scoped controllers; one sub-package per resource
│   │   │   └── setup.go               # registerFuncs slice — the controller registration list
│   │   └── global/                    # Cluster-scoped controllers
│   ├── api/                           # REST API handlers; one sub-package per resource
│   ├── webhook/
│   │   └── admission/                 # Admission webhook validators and mutators
│   ├── server/                        # HTTP server wiring; registers API routes and webhooks
│   ├── settings/                      # Harvester global settings (exported Setting vars)
│   ├── util/                          # Shared utilities; fake clients for unit tests
│   └── ...
├── deploy/charts/                     # Helm charts (harvester, harvester-crd)
├── package/                           # Docker packaging scripts and upgrade helper assets
├── scripts/                           # Build scripts (version, package, test-integration, etc.)
├── tests/
│   ├── framework/                     # Integration test helpers and cluster setup
│   └── integration/                   # Integration test suites (run via make test-integration)
├── docs/assets/                       # Documentation images referenced from README
├── enhancements/                      # HEPs (Harvester Enhancement Proposals)
└── hack/                              # Developer helper scripts
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go (version in `go.mod`) |
| Controller framework | Rancher Wrangler v3 (`github.com/rancher/wrangler/v3`) |
| Kubernetes client | `k8s.io/client-go` + Wrangler-generated typed clients |
| VM management | KubeVirt (`kubevirt.io/api`) |
| Storage | Longhorn (`longhorn.io`) |
| Cluster management | Rancher (`github.com/rancher/rancher`) |
| Networking | Multus CNI, Whereabouts, Harvester Network Controller |
| Linter | golangci-lint v2 (config: `.golangci.yaml`) |
| Build system | Docker multi-stage builds via `Makefile` |
| Dependency management | Vendored (`vendor/`) + Renovate (`renovate.json`) |
| CI | GitHub Actions (`.github/workflows/`) |

## Build & Run

All build targets run **inside Docker** — a Docker-compatible engine is the only host requirement.

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

## Testing

- **Unit tests**: `*_test.go` files alongside source. Use `testify/assert`. Fake clients in `pkg/util/fakeclients/`.
- **Integration tests**: `tests/integration/`. Run via `make test-integration` (requires Docker; spawns a kind cluster).
- **Reference pattern**: `pkg/controller/master/supportbundle/controller_test.go`

```bash
# Run a specific unit test package
go test ./pkg/controller/master/supportbundle/... -run TestCheckExistTime -v
```

## Key Patterns and Conventions

### Controller Pattern

Controllers live in `pkg/controller/{scope}/{resource}/` and follow this structure:

1. **Handler struct** — typed controller/client references obtained from `config.Management`
2. **Register function** — pulls typed controllers from `management.*Factory`, builds `Handler`, registers callbacks
3. **Callbacks** — `OnChange` and `OnRemove` on the relevant controller

```go
// pkg/controller/master/mything/handler.go
type Handler struct {
    myThings ctlharvesterv1.MyThingController
    nodes    ctlcorev1.NodeController
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
    myThingController := management.HarvesterFactory.Harvesterhci().V1beta1().MyThing()
    nodeController := management.CoreFactory.Core().V1().Node()

    h := &Handler{
        myThings: myThingController,
        nodes:    nodeController,
    }

    myThingController.OnChange(ctx, "my-thing-sync", h.OnChange)
    return nil
}
```

### API Handler Pattern

REST handlers live in `pkg/api/{resource}/`. Each sub-package registers routes through `pkg/server/` and uses typed clients from `config.Management`.

### Webhook Pattern

Admission webhooks live in `pkg/webhook/admission/`. Validators implement the `admission.Validator` interface; mutators implement `admission.Mutator`. Register in the webhook server setup.

### CRD Type Pattern (adding a new resource)

1. Define the type in `pkg/apis/harvesterhci.io/v1beta1/{resource}.go` with codegen markers
2. Run `go generate ./...` — regenerates clients, deepcopy, and register files
3. Add controller in `pkg/controller/master/{resource}/`
4. Register the controller's `Register` func in `pkg/controller/master/setup.go`
5. Add REST handler in `pkg/api/{resource}/` and register routes in `pkg/server/` (if REST API needed)
6. Add webhook in `pkg/webhook/admission/{resource}/` and register in webhook server (if validation needed)
7. Update Helm CRD chart: `deploy/charts/harvester-crd/`

## CI/CD

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `build.yml` | push / PR | Build and unit test |
| `codeql-analysis.yml` | schedule / PR | Security code scanning |
| `scan.yml` | schedule | Container image vulnerability scan |
| `fossa.yml` | push | License compliance scan |
| `validate-ci` | PR | Dirty-check after `go generate` + `go mod tidy` |

## Common Pitfalls

- **Never edit `pkg/generated/`** — fully regenerated by `go generate ./...`; edits will be overwritten
- **`vendor/` is committed** — after any `go.mod` change, run `go mod tidy && go mod vendor` and commit all three (`go.mod`, `go.sum`, `vendor/`)
- **`go.mod` replace directives are required** — they pin Rancher/Wrangler forks; do not remove them
- **Docker is required for `make build/test`** — all Makefile targets wrap commands in `docker build`
- **Signed-off commits required** — every commit needs `Signed-off-by: Name <email>` (use `git commit -s`)
- **PRs must link to an issue** — use `Issue #N`, not `Fixes #N` (which auto-closes issues on merge)
- **Target `master` branch** — the default and primary development branch is `master`
- **AI use must be disclosed** — if AI tools were used in authoring a PR, disclose it in the PR description
- **`scripts/.version_env`** — must be generated on a host with git access before running Docker builds in a worktree (`make gen-version-env` or `scripts/version`)

## Documentation

- **Official docs**: <https://docs.harvesterhci.io/> (source: <https://github.com/harvester/docs>)
- **Knowledge base**: <https://harvesterhci.io/kb/> (source: <https://github.com/harvester/harvesterhci.io>)
- **Local assets**: `docs/assets/` — images referenced in the README
- **Enhancement proposals**: `enhancements/` — HEP documents for larger feature work
