#!/usr/bin/env bash

##
# Inspired by github.com/kubernetes/kubernetes/hack/lib/version.sh
##

# -----------------------------------------------------------------------------
# Version management helpers. These functions help to set the
# following variables:
#
#        GIT_COMMIT  -  The git commit id corresponding to this
#                       source code.
#    GIT_TREE_STATE  -  "clean" indicates no changes since the git commit id
#                       "dirty" indicates source code changes after the git commit id
#                       "archive" indicates the tree was produced by 'git archive'
#       GIT_VERSION  -  "vX.Y" used to indicate the last release version.
#        BUILD_DATE  -  The build date of the version

function vm::version::get_version_vars() {
  BUILD_DATE=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

  # if the source was exported through git archive, then
  # use this to get the git tree state
  # shellcheck disable=SC2016,SC2050
  if [[ '$Format:%%$' == "%" ]]; then
    GIT_COMMIT='$Format:%H$'
    GIT_TREE_STATE="archive"
    # when a 'git archive' is exported, the '$Format:%D$' below will look
    # something like 'HEAD -> release-1.8, tag: v1.8.3' where then 'tag: '
    # can be extracted from it.
    if [[ '$Format:%D$' =~ tag:\ (v[^ ,]+) ]]; then
      GIT_VERSION="${BASH_REMATCH[1]}"
    fi
  fi

  # return if git hasn't installed
  if [[ -z "$(command -v git)" ]]; then
    GIT_COMMIT="unknown"
    GIT_TREE_STATE="unknown"
    GIT_VERSION=${DRONE_TAG:-unknown}
    return
  fi

  if [[ -n ${GIT_COMMIT:-} ]] || GIT_COMMIT=$(git rev-parse "HEAD^{commit}" 2>/dev/null); then
    if [[ -z ${GIT_TREE_STATE:-} ]]; then
      # check if the tree is dirty, default is dirty.
      if git_status=$(git status --porcelain 2>/dev/null) && [[ -n ${git_status} ]]; then
        GIT_TREE_STATE="dirty"
      else
        GIT_TREE_STATE="clean"
      fi
    fi
  fi

  if [[ -n ${GIT_VERSION:-} ]] || GIT_VERSION="$(git rev-parse --abbrev-ref HEAD | sed -E 's/[^a-zA-Z0-9]+/-/g' 2>/dev/null)"; then
    # check if the HEAD is tagged.
    if git_tag=$(git tag -l --contains HEAD | head -n 1 2>/dev/null) && [[ -n ${DRONE_TAG:-${git_tag}} ]]; then
      GIT_VERSION="${DRONE_TAG:-${git_tag}}"

      # refuse to build if tag is not a valid Semantic Version
      if ! [[ "${GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
        vm::log::fatal "GIT_VERSION should be a valid Semantic Version.
      Current value is: ${GIT_VERSION}
      Please see more details here: https://semver.org"
      fi
    fi

    # append suffix if the tree is dirty.
    if [[ "${GIT_TREE_STATE}" == "dirty" ]]; then
      GIT_VERSION+="-dirty"
    fi
  fi
}
