#!/bin/bash -e
# This script collects the chart and app versions of the Rancher dependencies
# that Harvester bundles (fleet, fleet-crd, rancher-webhook).
#
# It reads the authoritative versions straight from Rancher's upstream sources
# over HTTP, so it needs only network egress -- no Docker daemon and no need to
# pull the (large) Rancher image:
#
#   * chart versions come from rancher/rancher's build.yaml (the single source
#     of truth for the bundled dependency chart versions), and
#   * app versions come from each chart's Chart.yaml in rancher/charts.
#
# For example:
#
# $ <script_name> /tmp/test.yaml
#
# will generate a YAML file like:
#
# ```
# rancherDependencies:
#   fleet:
#     chart: 110.0.1+up0.16.1
#     app: "0.16.1"
#   fleet-crd:
#     chart: 110.0.1+up0.16.1
#     app: "0.16.1"
#   rancher-webhook:
#     chart: 110.0.2+up0.11.1
#     app: "0.11.1"
# ```

output_file=$1

TOP_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"
SCRIPTS_DIR="${TOP_DIR}/scripts"

source ${SCRIPTS_DIR}/version-rancher

RANCHER_RAW_BASE="https://raw.githubusercontent.com/rancher/rancher"
CHARTS_RAW_BASE="https://raw.githubusercontent.com/rancher/charts"

# Derive the rancher/charts branch from the Rancher version, e.g.
# "v2.15.1" -> "release-v2.15". Overridable via RANCHER_CHARTS_BRANCH.
charts_branch()
{
  if [ -n "${RANCHER_CHARTS_BRANCH:-}" ]; then
    echo "${RANCHER_CHARTS_BRANCH}"
    return
  fi

  local v="${RANCHER_VERSION#v}"
  local major="${v%%.*}"
  local rest="${v#*.}"
  local minor="${rest%%.*}"

  if [ -z "$major" ] || [ -z "$minor" ] || [ "$major" = "$v" ]; then
    echo "Cannot derive charts branch from RANCHER_VERSION '${RANCHER_VERSION}'" >&2
    return 1
  fi

  echo "release-v${major}.${minor}"
}

# fetch_url <url> <output_file>
# Downloads <url> to <output_file>, then sleeps briefly to be gentle to GitHub.
fetch_url()
{
  local url=$1
  local output_file=$2

  curl -fsSL "$url" -o "$output_file"
  sleep 0.5 # be gentle to GitHub
}

# update_chart_app_versions <branch> <name> <chart_version> <output_file>
# Fetches <name>/<chart_version>/Chart.yaml from rancher/charts, validates the
# chart version matches, and records the chart and app versions in output_file.
update_chart_app_versions()
{
  local branch=$1
  local name=$2
  local chart_version=$3
  local output_file=$4

  local chart_yaml
  chart_yaml=$(mktemp)

  local url="${CHARTS_RAW_BASE}/${branch}/charts/${name}/${chart_version}/Chart.yaml"
  echo "Reading ${name} ${chart_version}: ${url}"
  fetch_url "$url" "$chart_yaml"

  local actual_version
  actual_version="$(yq e '.version' "$chart_yaml")"
  if [ "$actual_version" != "$chart_version" ]; then
    echo "Chart version mismatch for ${name}: expected '${chart_version}', got '${actual_version}'" >&2
    rm -f "$chart_yaml"
    return 1
  fi

  local app_version
  app_version="$(yq e '.appVersion' "$chart_yaml")"
  if [ -z "$app_version" ] || [ "$app_version" = "null" ]; then
    echo "Missing appVersion for ${name} ${chart_version}" >&2
    rm -f "$chart_yaml"
    return 1
  fi

  # Quote the app version so numeric-looking values (e.g. 0.16.1) stay strings.
  NAME=$name CHART_VERSION=$chart_version APP_VERSION=$app_version \
    yq e '.rancherDependencies[strenv(NAME)].chart = strenv(CHART_VERSION) |
          .rancherDependencies[strenv(NAME)].app = strenv(APP_VERSION)' -i "$output_file"

  rm -f "$chart_yaml"
}

update_rancher_deps()
{
  local rancher_version=$1
  local output_file=$2

  local branch
  branch=$(charts_branch)

  local build_yaml
  build_yaml=$(mktemp)

  # build.yaml carries the exact chart versions of the bundled dependencies.
  local url="${RANCHER_RAW_BASE}/${rancher_version}/build.yaml"
  echo "Reading Rancher ${rancher_version} build.yaml: ${url}"
  fetch_url "$url" "$build_yaml"

  local fleet_version
  local webhook_version
  fleet_version="$(yq e '.fleetVersion' "$build_yaml")"
  webhook_version="$(yq e '.webhookVersion' "$build_yaml")"

  if [ -z "$fleet_version" ] || [ "$fleet_version" = "null" ]; then
    echo "Missing fleetVersion in ${url}" >&2
    rm -f "$build_yaml"
    return 1
  fi
  if [ -z "$webhook_version" ] || [ "$webhook_version" = "null" ]; then
    echo "Missing webhookVersion in ${url}" >&2
    rm -f "$build_yaml"
    return 1
  fi

  # fleet-crd is released in lockstep with fleet at the same chart version.
  update_chart_app_versions "$branch" fleet "$fleet_version" "$output_file"
  update_chart_app_versions "$branch" fleet-crd "$fleet_version" "$output_file"
  update_chart_app_versions "$branch" rancher-webhook "$webhook_version" "$output_file"

  rm -f "$build_yaml"
}


if [ ! -e $output_file ]; then
  touch $output_file
fi

update_rancher_deps "$RANCHER_VERSION" "$output_file"

echo "Rancher dependencies:"
cat "$output_file"
