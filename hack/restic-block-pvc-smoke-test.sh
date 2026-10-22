#!/usr/bin/env bash
# Example:
#
#   IMAGE=10.115.54.34:5000/harvester:issue-10567-head \
#   PVC_SIZE=1Gi \
#   PAYLOAD_MIB=64 \
#   KEEP=true \
#   bash hack/restic-block-pvc-smoke-test.sh
#
# Remove Kubernetes resources and remote snapshots left by previous runs:
#
#   bash hack/restic-block-pvc-smoke-test.sh cleanup
#
set -euo pipefail

MODE="${1:-${MODE:-run}}"
if [[ "${MODE}" != "run" && "${MODE}" != "cleanup" ]]; then
	echo "usage: $0 [run|cleanup]" >&2
	exit 1
fi

NAMESPACE="${NAMESPACE:-longhorn-system}"
SECRET_NAME="${SECRET_NAME:-harvester-backup-target-secret}"
BACKUP_TARGET_SETTING="${BACKUP_TARGET_SETTING:-backup-target}"
IMAGE="${IMAGE:-10.115.54.34:5000/harvester:issue-10567-head}"
RESTIC_BUCKET="${RESTIC_BUCKET:-}"
RESTIC_ENDPOINT="${RESTIC_ENDPOINT:-}"
RESTIC_REGION="${RESTIC_REGION:-}"
RESTIC_VIRTUAL_HOSTED_STYLE="${RESTIC_VIRTUAL_HOSTED_STYLE:-}"
PVC_SIZE="${PVC_SIZE:-1Gi}"
PAYLOAD_MIB="${PAYLOAD_MIB:-64}"
STORAGE_CLASS="${STORAGE_CLASS:-}"
TIMEOUT="${TIMEOUT:-30m}"
KEEP="${KEEP:-false}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)}"

SOURCE_PVC="${SOURCE_PVC:-restic-source-${RUN_ID}}"
TARGET_PVC="${TARGET_PVC:-restic-target-${RUN_ID}}"
WRITE_JOB="${WRITE_JOB:-restic-write-${RUN_ID}}"
BACKUP_JOB="${BACKUP_JOB:-restic-backup-${RUN_ID}}"
RESTORE_JOB="${RESTORE_JOB:-restic-restore-${RUN_ID}}"
VMBACKUP_TAG="${VMBACKUP_TAG:-restic-smoke-${RUN_ID}}"
SNAPSHOT_TAG="${SNAPSHOT_TAG:-${SOURCE_PVC}}"
SMOKE_TEST_LABEL="harvesterhci.io/smoke-test"
SMOKE_TEST_VALUE="restic-block-pvc"
SMOKE_TEST_TAG="smoke:restic-block-pvc"
CLEANUP_JOB="${CLEANUP_JOB:-restic-cleanup-${RUN_ID}}"

for command in kubectl jq sha256sum; do
	if ! command -v "${command}" >/dev/null 2>&1; then
		echo "required command not found: ${command}" >&2
		exit 1
	fi
done

sha256_hex() {
	local output
	output="$(sha256sum)"
	printf '%s' "${output%% *}"
}

BACKUP_TARGET_VALUE="$(kubectl get settings.harvesterhci.io "${BACKUP_TARGET_SETTING}" -o jsonpath='{.value}')"
BACKUP_TARGET_TYPE="$(jq -er '.type' <<<"${BACKUP_TARGET_VALUE}")"
if [[ "${BACKUP_TARGET_TYPE}" != "s3" ]]; then
	echo "Restic smoke test requires an S3 backup target, got: ${BACKUP_TARGET_TYPE}" >&2
	exit 1
fi

if [[ -z "${RESTIC_BUCKET}" ]]; then
	RESTIC_BUCKET="$(jq -er '.bucketName | select(length > 0)' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${RESTIC_ENDPOINT}" ]]; then
	RESTIC_ENDPOINT="$(jq -r '.endpoint // ""' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${RESTIC_REGION}" ]]; then
	RESTIC_REGION="$(jq -r '.bucketRegion // ""' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${RESTIC_VIRTUAL_HOSTED_STYLE}" ]]; then
	RESTIC_VIRTUAL_HOSTED_STYLE="$(jq -r '.virtualHostedStyle // false' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ "${RESTIC_VIRTUAL_HOSTED_STYLE}" != "true" && "${RESTIC_VIRTUAL_HOSTED_STYLE}" != "false" ]]; then
	echo "RESTIC_VIRTUAL_HOSTED_STYLE must be true or false" >&2
	exit 1
fi
while [[ "${RESTIC_ENDPOINT}" == */ ]]; do
	RESTIC_ENDPOINT="${RESTIC_ENDPOINT%/}"
done
REPOSITORY_ID="sha256:$(
	printf '%s\0%s\0%s\0%s\0%s' \
		"${BACKUP_TARGET_TYPE}" \
		"${RESTIC_ENDPOINT}" \
		"${RESTIC_BUCKET}" \
		"${RESTIC_REGION}" \
		"${RESTIC_VIRTUAL_HOSTED_STYLE}" | sha256_hex
)"
RESTIC_REPOSITORY_PASSWORD="$(
	printf '%s\0%s' "restic" "${REPOSITORY_ID}" | sha256_hex
)"
if [[ -z "${RESTIC_ENDPOINT}" ]]; then
	RESTIC_ENDPOINT="s3.amazonaws.com"
fi
RESTIC_REPOSITORY="s3:${RESTIC_ENDPOINT}/${RESTIC_BUCKET}/restic"

print_pvc() {
	local name="$1"
	cat <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${name}
  namespace: ${NAMESPACE}
  labels:
    ${SMOKE_TEST_LABEL}: ${SMOKE_TEST_VALUE}
spec:
EOF
	if [[ -n "${STORAGE_CLASS}" ]]; then
		printf '  storageClassName: %s\n' "${STORAGE_CLASS}"
	fi
	cat <<EOF
  accessModes:
  - ReadWriteOnce
  volumeMode: Block
  resources:
    requests:
      storage: ${PVC_SIZE}
EOF
}

cleanup_current_run() {
	if [[ "${KEEP}" == "true" ]]; then
		echo "KEEP=true, leaving resources in ${NAMESPACE}"
		return
	fi
	kubectl -n "${NAMESPACE}" delete job "${WRITE_JOB}" "${BACKUP_JOB}" "${RESTORE_JOB}" --ignore-not-found
	kubectl -n "${NAMESPACE}" delete pvc "${SOURCE_PVC}" "${TARGET_PVC}" --ignore-not-found
}

cleanup_smoke_test_resources() {
	local kind
	local -a names
	for kind in job pvc; do
		mapfile -t names < <(
			kubectl -n "${NAMESPACE}" get "${kind}" -o json | jq -r \
				--arg label_key "${SMOKE_TEST_LABEL}" \
				--arg value "${SMOKE_TEST_VALUE}" \
				--arg kind "${kind}" '
				.items[] |
				select(
					.metadata.labels[$label_key] == $value or
					(if $kind == "job" then
						(.metadata.name | test("^restic-(write|backup|restore|cleanup)-"))
					 else
						(.metadata.name | test("^restic-(source|target)-"))
					 end)
				) |
				.metadata.name'
		)
		if ((${#names[@]} > 0)); then
			kubectl -n "${NAMESPACE}" delete "${kind}" "${names[@]}" --ignore-not-found
		fi
	done
}

wait_job() {
	local job="$1"
	local complete_pid failed_pid succeeded
	kubectl -n "${NAMESPACE}" wait --for=condition=complete "job/${job}" --timeout="${TIMEOUT}" >/dev/null 2>&1 &
	complete_pid=$!
	kubectl -n "${NAMESPACE}" wait --for=condition=failed "job/${job}" --timeout="${TIMEOUT}" >/dev/null 2>&1 &
	failed_pid=$!
	wait -n "${complete_pid}" "${failed_pid}" || true
	kill "${complete_pid}" "${failed_pid}" 2>/dev/null || true
	wait "${complete_pid}" "${failed_pid}" 2>/dev/null || true

	succeeded="$(kubectl -n "${NAMESPACE}" get "job/${job}" -o jsonpath='{.status.succeeded}')"
	if [[ "${succeeded}" == "1" ]]; then
		return
	fi
	echo "Job ${job} failed or timed out. Recent logs:"
	kubectl -n "${NAMESPACE}" logs "job/${job}" --all-containers=true --tail=-1 || true
	return 1
}

job_logs() {
	kubectl -n "${NAMESPACE}" logs "job/$1" --all-containers=true --tail=-1
}

cleanup_previous_runs() {
	local remote_cleanup_status=0
	# Stop old jobs before scanning the repository so they cannot create a new
	# snapshot after the remote cleanup has finished.
	cleanup_smoke_test_resources
	cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${CLEANUP_JOB}
  namespace: ${NAMESPACE}
  labels:
    ${SMOKE_TEST_LABEL}: ${SMOKE_TEST_VALUE}
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: cleanup
        image: ${IMAGE}
        command: ["/bin/sh", "-c"]
        env:
        - name: RESTIC_REPOSITORY
          value: "${RESTIC_REPOSITORY}"
        - name: RESTIC_S3_REGION
          value: "${RESTIC_REGION}"
        - name: RESTIC_CACHE_DIR
          value: /restic-cache
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_ACCESS_KEY_ID
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_SECRET_ACCESS_KEY
        - name: RESTIC_PASSWORD
          value: "${RESTIC_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          restic_cmd() {
            if [ -n "\${RESTIC_S3_REGION}" ]; then
              restic -o "s3.region=\${RESTIC_S3_REGION}" "\$@"
            else
              restic "\$@"
            fi
          }
          IDS=\$(restic_cmd snapshots --json --tag 'ns:${NAMESPACE}' | jq -r '
            .[] |
            select(
              ((.tags // []) | index("${SMOKE_TEST_TAG}")) or
              ((.tags // []) | index("vmb:${VMBACKUP_TAG}")) or
              ((.tags // []) | any(startswith("vmb:restic-smoke-")))
            ) |
            .id
          ')
          if [ -n "\${IDS}" ]; then
            # Snapshot IDs contain no whitespace, so intentional word splitting is safe here.
            restic_cmd forget --prune \${IDS}
          else
            echo 'no matching Restic smoke-test snapshots found'
          fi
        volumeMounts:
        - name: restic-cache
          mountPath: /restic-cache
      volumes:
      - name: restic-cache
        emptyDir:
          sizeLimit: 2Gi
EOF

	if ! wait_job "${CLEANUP_JOB}"; then
		remote_cleanup_status=1
	fi
	kubectl -n "${NAMESPACE}" delete job "${CLEANUP_JOB}" --ignore-not-found
	if ((remote_cleanup_status != 0)); then
		echo "remote Restic snapshot cleanup failed" >&2
		return "${remote_cleanup_status}"
	fi
	echo "removed Restic smoke-test snapshots and Kubernetes resources from ${NAMESPACE}"
}

if [[ "${MODE}" == "cleanup" ]]; then
	cleanup_previous_runs
	exit
fi

trap cleanup_current_run EXIT

{
	print_pvc "${SOURCE_PVC}"
	echo "---"
	print_pvc "${TARGET_PVC}"
} | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${WRITE_JOB}
  namespace: ${NAMESPACE}
  labels:
    ${SMOKE_TEST_LABEL}: ${SMOKE_TEST_VALUE}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: write
        image: ${IMAGE}
        command: ["/bin/sh", "-c"]
        args:
        - |
          set -eo pipefail
          dd if=/dev/urandom of=/dev/source bs=1M count=${PAYLOAD_MIB} conv=fsync
          sync
          HASH=\$(sha256sum /dev/source)
          HASH=\${HASH%% *}
          echo "SOURCE_SHA256 \${HASH}"
        volumeDevices:
        - name: source
          devicePath: /dev/source
      volumes:
      - name: source
        persistentVolumeClaim:
          claimName: ${SOURCE_PVC}
EOF

wait_job "${WRITE_JOB}"
SOURCE_PV="$(kubectl -n "${NAMESPACE}" get pvc "${SOURCE_PVC}" -o jsonpath='{.spec.volumeName}')"
if [[ -z "${SOURCE_PV}" ]]; then
	echo "failed to resolve PV backing source PVC ${NAMESPACE}/${SOURCE_PVC}" >&2
	exit 1
fi
SOURCE_HASH=""
while read -r marker hash _; do
	if [[ "${marker}" == "SOURCE_SHA256" ]]; then
		SOURCE_HASH="${hash}"
	fi
done < <(job_logs "${WRITE_JOB}")
if [[ -z "${SOURCE_HASH}" ]]; then
	echo "failed to read source hash from ${WRITE_JOB} logs"
	exit 1
fi
echo "source hash: ${SOURCE_HASH}"

cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${BACKUP_JOB}
  namespace: ${NAMESPACE}
  labels:
    ${SMOKE_TEST_LABEL}: ${SMOKE_TEST_VALUE}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: backup
        image: ${IMAGE}
        command: ["/bin/sh", "-c"]
        resources:
          requests:
            ephemeral-storage: 512Mi
          limits:
            ephemeral-storage: 3Gi
        env:
        - name: RESTIC_REPOSITORY
          value: "${RESTIC_REPOSITORY}"
        - name: RESTIC_S3_REGION
          value: "${RESTIC_REGION}"
        - name: RESTIC_CACHE_DIR
          value: /restic-cache
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_ACCESS_KEY_ID
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_SECRET_ACCESS_KEY
        - name: RESTIC_PASSWORD
          value: "${RESTIC_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          restic_cmd() {
            if [ -n "\${RESTIC_S3_REGION}" ]; then
              restic -o "s3.region=\${RESTIC_S3_REGION}" "\$@"
            else
              restic "\$@"
            fi
          }
          (restic_cmd snapshots > /dev/null 2>&1 || { restic_cmd init || restic_cmd snapshots > /dev/null 2>&1; })
          /usr/bin/harvester io-mode -device /dev/source -mode=read | \
            restic_cmd -q backup --stdin --stdin-filename ${SOURCE_PV} --tag=ns:${NAMESPACE},vmb:${VMBACKUP_TAG},sn:${SNAPSHOT_TAG},${SMOKE_TEST_TAG}
        volumeMounts:
        - name: restic-cache
          mountPath: /restic-cache
        volumeDevices:
        - name: source
          devicePath: /dev/source
      volumes:
      - name: restic-cache
        emptyDir:
          sizeLimit: 2Gi
      - name: source
        persistentVolumeClaim:
          claimName: ${SOURCE_PVC}
EOF

wait_job "${BACKUP_JOB}"

cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${RESTORE_JOB}
  namespace: ${NAMESPACE}
  labels:
    ${SMOKE_TEST_LABEL}: ${SMOKE_TEST_VALUE}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: restore
        image: ${IMAGE}
        command: ["/bin/sh", "-c"]
        resources:
          requests:
            ephemeral-storage: 512Mi
          limits:
            ephemeral-storage: 3Gi
        env:
        - name: RESTIC_REPOSITORY
          value: "${RESTIC_REPOSITORY}"
        - name: RESTIC_S3_REGION
          value: "${RESTIC_REGION}"
        - name: RESTIC_CACHE_DIR
          value: /restic-cache
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_ACCESS_KEY_ID
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: ${SECRET_NAME}
              key: AWS_SECRET_ACCESS_KEY
        - name: RESTIC_PASSWORD
          value: "${RESTIC_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          restic_cmd() {
            if [ -n "\${RESTIC_S3_REGION}" ]; then
              restic -o "s3.region=\${RESTIC_S3_REGION}" "\$@"
            else
              restic "\$@"
            fi
          }
          restic_cmd -q dump --tag=ns:${NAMESPACE},vmb:${VMBACKUP_TAG},sn:${SNAPSHOT_TAG} latest /${SOURCE_PV} | \
            /usr/bin/harvester io-mode -device /dev/target -mode=write
          sync
          HASH=\$(sha256sum /dev/target)
          HASH=\${HASH%% *}
          echo "TARGET_SHA256 \${HASH}"
          if [ "\${HASH}" != "${SOURCE_HASH}" ]; then
            echo "data integrity check failed: source=${SOURCE_HASH} target=\${HASH}" >&2
            exit 1
          fi
          if [ "${KEEP}" != "true" ]; then
            restic_cmd forget --tag ns:${NAMESPACE},vmb:${VMBACKUP_TAG},sn:${SNAPSHOT_TAG} --unsafe-allow-remove-all --prune
          fi
        volumeMounts:
        - name: restic-cache
          mountPath: /restic-cache
        volumeDevices:
        - name: target
          devicePath: /dev/target
      volumes:
      - name: restic-cache
        emptyDir:
          sizeLimit: 2Gi
      - name: target
        persistentVolumeClaim:
          claimName: ${TARGET_PVC}
EOF

wait_job "${RESTORE_JOB}"
TARGET_HASH=""
while read -r marker hash _; do
	if [[ "${marker}" == "TARGET_SHA256" ]]; then
		TARGET_HASH="${hash}"
	fi
done < <(job_logs "${RESTORE_JOB}")
if [[ -z "${TARGET_HASH}" ]]; then
	echo "failed to read target hash from ${RESTORE_JOB} logs"
	exit 1
fi
echo "target hash: ${TARGET_HASH}"

if [[ "${SOURCE_HASH}" != "${TARGET_HASH}" ]]; then
	echo "data integrity check failed"
	echo "source=${SOURCE_HASH}"
	echo "target=${TARGET_HASH}"
	exit 1
fi

echo "restic block PVC smoke test passed"
echo "namespace=${NAMESPACE}"
echo "source_pvc=${SOURCE_PVC}"
echo "source_pv=${SOURCE_PV}"
echo "target_pvc=${TARGET_PVC}"
echo "repository=${RESTIC_REPOSITORY} region=${RESTIC_REGION:-<default>}"
echo "tags=ns:${NAMESPACE},vmb:${VMBACKUP_TAG},sn:${SNAPSHOT_TAG}"
