#!/bin/bash
set -e

UPGRADE_MATRIX_FILE=$1
IMAGES_LISTS_DIR=$2
RANCHERD_IMAGES_DIR=$3
IMAGES_LISTS_ARCHIVE_DIR=$4
ARCH=$5

WORKING_DIR=$(mktemp -d)

version_compare() {
    local v1=$1
    local v2=$2
    if [[ "$(printf "%s\n" "$v1" "$v2" | sort -V | head -n1)" == "$v2" ]]; then
        echo "1"  # v1 is greater than or equal to v2
    else
        echo "0"  # v1 is less than v2
    fi
}

if [ ! -e "$UPGRADE_MATRIX_FILE" ]; then
  echo "Could not find $UPGRADE_MATRIX_FILE, skip it"
  exit 0
fi

# Add all the current version's images lists
mkdir -p "$IMAGES_LISTS_ARCHIVE_DIR"/current
find $IMAGES_LISTS_DIR -name "*.txt" -exec cat {} \; >> "$IMAGES_LISTS_ARCHIVE_DIR"/current/image_list_all.txt
find $RANCHERD_IMAGES_DIR -name "*.txt" -exec cat {} \; >> "$IMAGES_LISTS_ARCHIVE_DIR"/current/image_list_all.txt
cat "$IMAGES_LISTS_ARCHIVE_DIR"/current/image_list_all.txt | sort | uniq | tee "$IMAGES_LISTS_ARCHIVE_DIR"/current/image_list_all.txt

# Add all the previous versions' images lists
all_previous_versions=$(yq e ".versions[].name" "$UPGRADE_MATRIX_FILE" | xargs)
if [[ "$ARCH" == "arm64" ]]; then
  # ARM64 is supported in v1.3.0 and later versions
  arm_previous_versions=()
  for version in $(echo "$all_previous_versions"); do
      if [[ "$(version_compare "$version" "v1.3.0")" == "1" ]]; then
          echo "ARM64 image is supported after v1.3.0, skip $version"
          arm_previous_versions+=("$version")
      fi
  done
  previous_versions=$(IFS=" "; echo "${arm_previous_versions[*]}")
else
  # AMD64 is supported for all versions
  previous_versions=$all_previous_versions
fi

for prev_ver in $(echo "$previous_versions"); do
  ret=0

  echo "Fetching $prev_ver image lists..."

  mkdir "$WORKING_DIR"/"$prev_ver"

  image_lists_url=https://releases.rancher.com/harvester/"$prev_ver"/image-lists.tar.gz
  # arch-aware image is available after v1.3.0
  if [ "$(version_compare "$prev_ver" "v1.3.0")" = "1" ]; then
    image_lists_url=https://releases.rancher.com/harvester/"$prev_ver"/image-lists-"$ARCH".tar.gz
  fi

  echo "Download image lists tarball from $image_lists_url"
  curl -fL "$image_lists_url" -o "$WORKING_DIR"/image-lists.tar.gz || ret=$?
  if [ $ret -ne 0 ]; then
    echo "Cannot download image list tarball for version $prev_ver, skip it"
    continue
  fi
  tar -zxvf "$WORKING_DIR"/image-lists.tar.gz -C "$WORKING_DIR"/"$prev_ver"/

  prev_image_list="$WORKING_DIR"/image_list_all.txt
  cat "$WORKING_DIR"/"$prev_ver"/image-lists/*.txt | sort | uniq > "$prev_image_list"

  mkdir -p "$IMAGES_LISTS_ARCHIVE_DIR"/"$prev_ver"
  cp -a "$prev_image_list" "$IMAGES_LISTS_ARCHIVE_DIR"/"$prev_ver"/

  rm -rf "$WORKING_DIR"/*
done
