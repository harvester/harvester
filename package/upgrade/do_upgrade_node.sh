#!/bin/bash -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

retries=0

until $SCRIPT_DIR/upgrade_node.sh $@; do
  # Do not fail pre-drain/post-drain jobs
  case $1 in
  "pre-drain"|"post-drain")
    ;;
  *)
    echo "[Error] Running \"upgrade_node.sh $*\""
    exit 1
    ;;
  esac

  # A fallback to exit if seeing some files
  # This could be useful to modify a running job container and resume an upgrade.
  if [ -e "/tmp/skip-retry-with-fail" ]; then
    echo "Exit 1 manually."
    exit 1
  fi

  if [ -e "/tmp/skip-retry-with-succeed" ]; then
    echo "Exit 0 manually."
    exit 0
  fi

  echo "[$(date)] Running \"upgrade_node.sh $*\" errors, will retry after 10 minutes ($retries retries)..."
  sleep 10m
  retries=$((retries+1))
done
