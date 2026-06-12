#!/bin/bash -x
function list_units() {
  systemctl list-units --type=scope --no-legend | grep -Eo "(cri-containerd-.+\.scope)"
}

systemctl stop rke2-server.service || true
systemctl stop rke2-agent.service || true

unit_list=($(list_units))

max_concurrency=$(($(nproc) + 1))
if [ ${#unit_list[@]} -lt $max_concurrency ]; then
    max_concurrency=${#unit_list[@]}
fi

printf '%s\0' "${unit_list[@]}" | xargs -0 -n1 -P "$max_concurrency" sh -c 'sudo systemctl stop "$@" && echo "Finished $@"' _ | { grep -v '^Finished' || true; } | { grep -v 'xargs:' || true; } &
wait

echo "Finished stopping all units."
exit 0