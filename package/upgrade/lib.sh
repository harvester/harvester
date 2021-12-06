
UPGRADE_REPO_URL=http://upgrade-repo-$HARVESTER_UPGRADE_NAME.harvester-system/harvester-iso
UPGRADE_REPO_RELEASE_FILE="$UPGRADE_REPO_URL/harvester-release.yaml"
UPGRADE_REPO_SQUASHFS_IMAGE="$UPGRADE_REPO_URL/rootfs.squashfs"
UPGRADE_REPO_BUNDLE_ROOT="$UPGRADE_REPO_URL/bundle"
UPGRADE_REPO_BUNDLE_METADATA="$UPGRADE_REPO_URL/bundle/metadata.yaml"

detect_repo()
{
  release_file=$(mktemp --suffix=.yaml)
  curl -sfL $UPGRADE_REPO_RELEASE_FILE -o $release_file

  REPO_HARVESTER_VERSION=$(yq -e e '.harvester' $release_file)
  REPO_HARVESTER_CHART_VERSION=$(yq -e e '.harvesterChart' $release_file)
  REPO_OS_PRETTY_NAME="$(yq -e e '.os' $release_file)"
  REPO_OS_VERSION="${REPO_OS_PRETTY_NAME#Harvester }"
  REPO_RKE2_VERSION=$(yq -e e '.kubernetes' $release_file)
  REPO_RANCHER_VERSION=$(yq -e e '.rancher' $release_file)
  REPO_MONITORING_CHART_VERSION=$(yq -e e '.monitoringChart' $release_file)

  if [ -z "$REPO_HARVESTER_VERSION" ]; then
    echo "[ERROR] Fail to get Harvester version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_HARVESTER_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get Harvester chart version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_OS_VERSION" ]; then
    echo "[ERROR] Fail to get OS version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RKE2_VERSION" ]; then
    echo "[ERROR] Fail to get RKE2 version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_RANCHER_VERSION" ]; then
    echo "[ERROR] Fail to get Rancher version from upgrade repo."
    exit 1
  fi

  if [ -z "$REPO_MONITORING_CHART_VERSION" ]; then
    echo "[ERROR] Fail to get monitoring chart version from upgrade repo."
    exit 1
  fi
}

wait_repo()
{
  until curl -sfL $UPGRADE_REPO_RELEASE_FILE
  do
    echo "Wait for upgrade repo ready..."
    sleep 5
  done
}

download_image_archives_from_repo() {
  local image_type=$1
  local save_dir=$2

  local metadata
  local image_list_url
  local archive_url
  local image_list_file
  local archive_file

  metadata=$(mktemp --suffix=.yaml)
  curl -fL $UPGRADE_REPO_BUNDLE_METADATA -o $metadata

  yq -e -o=json e ".images.$image_type" $metadata | jq -r '.[] | [.list, .archive] | @tsv' |
    while IFS=$'\t' read -r list archive; do
      image_list_url="$UPGRADE_REPO_BUNDLE_ROOT/$list"
      archive_url="$UPGRADE_REPO_BUNDLE_ROOT/$archive"
      image_list_file="$save_dir/$(basename $list)"
      archive_file="$save_dir/$(basename $archive)"

      if [ ! -e $image_list_file ]; then
        curl -fL $image_list_url -o $image_list_file
      fi

      if [ ! -e $archive_file ]; then
        curl -fL $archive_url -o $archive_file
      fi
    done
  
  rm -f $metadata
}
