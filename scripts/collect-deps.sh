#!/bin/bash -e
# This script collets versions of various components for update.
# For example:
#
# $ <script_name> /tmp/test.yaml
#
# will generate a YAML file like:
#
# ```
# rancherDependencies:
#   fleet:
#     chart: 100.0.3+up0.3.9-rc6
#     app: 0.3.9-rc6
#   fleet-crd:
#     chart: 100.0.3+up0.3.9-rc6
#     app: 0.3.9-rc6
#   rancher-webhook:
#     chart: 1.0.4+up0.2.5-rc3
#     app: 0.2.5-rc3
# ```

output_file=$1

TOP_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"
SCRIPTS_DIR="${TOP_DIR}/scripts"
WORKING_DIR=$(mktemp -d)

source ${SCRIPTS_DIR}/version-rancher

update_chart_app_versions()
{
  local index_file=$1
  local name=$2
  local min_version=$3
  local output_file=$4
  local chart_version
  local app_version

  local versions=$(mktemp)

  yq e ".entries.${name}[].version" $index_file > $versions
  echo $min_version >> $versions
  chart_version="$(sort -V -r $versions | head -n1)"
  app_version="$(CHART_VERSION=$chart_version yq e ".entries.${name}[] | select(.version == strenv(CHART_VERSION)) | .appVersion" $index_file)"

  yq e ".rancherDependencies.$name.chart = \"$chart_version\"" -i $output_file
  yq e ".rancherDependencies.$name.app = \"$app_version\"" -i $output_file
}

update_rancher_deps()
{
  local rancher_version=$1
  local output_file=$2

  local repo_hash
  local cid_file="${WORKING_DIR}/rancher-cid"
  local repo_index="${WORKING_DIR}/rancher-charts.yaml"
  local fleet_versions="${WORKING_DIR}/fleet-versions.txt"
  local webhook_versions="${WORKING_DIR}/webhook-versions.txt"
  local rancher_build_yaml="${WORKING_DIR}/rancher-build.yaml"
  local rancher_image="rancher/rancher:$rancher_version"

  # Get min verseion from rancher image's env variables
  curl https://raw.githubusercontent.com/rancher/rancher/$rancher_version/build.yaml -o $rancher_build_yaml
  CATTLE_FLEET_MIN_VERSION=$(yq e .fleetVersion $rancher_build_yaml)
  CATTLE_RANCHER_WEBHOOK_MIN_VERSION=$(yq e .webhookVersion $rancher_build_yaml)

  # Extract rancher-charts helm index file
  repo_hash=$(docker run --rm --entrypoint=/bin/bash "$rancher_image" -c "ls /var/lib/rancher-data/local-catalogs/v2/rancher-charts | head -n1")
  docker run --privileged -d --cidfile="$cid_file" "$rancher_image"
  sleep 10
  docker cp $(<$cid_file):/var/lib/rancher-data/local-catalogs/v2/rancher-charts/$repo_hash/index.yaml $repo_index
  docker stop $(<$cid_file)
  docker rm $(<$cid_file)

  # Get the latest version >= min_version and update it to yaml file
  update_chart_app_versions $repo_index fleet $CATTLE_FLEET_MIN_VERSION $output_file
  update_chart_app_versions $repo_index fleet-crd $CATTLE_FLEET_MIN_VERSION $output_file
  update_chart_app_versions $repo_index rancher-webhook $CATTLE_RANCHER_WEBHOOK_MIN_VERSION $output_file
}


if [ ! -e $output_file ]; then
  touch $output_file
fi

update_rancher_deps "$RANCHER_VERSION" "$output_file"

rm -rf $WORKING_DIR
