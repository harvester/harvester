#!/bin/bash -eu
# Verify that every image in the supplied lists is present in containerd.

if [[ $# -eq 0 ]]; then
    echo "Usage: $0 IMAGE_LIST [IMAGE_LIST ...]" >&2
    exit 1
fi

sorted_list_file=$(mktemp)
sort -u "$@" > "$sorted_list_file"

trap 'rm -f "$sorted_list_file"' EXIT

lines=$(wc -l < "$sorted_list_file")
echo "Checking $lines imported images..."

missing=$(ctr -n k8s.io images ls -q | grep -v '^sha256' | sort -u | comm -23 "$sorted_list_file" -)
if [[ -n "$missing" ]]; then
    echo "The following images are missing from containerd:" >&2
    echo "$missing" >&2
    exit 1
fi

echo done
