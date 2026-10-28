#!/usr/bin/env bash
# Example:
#
#   IMAGE=10.115.54.34:5000/harvester:issue-10567-head \
#   PVC_SIZE=1Gi \
#   PAYLOAD_MIB=64 \
#   KEEP=true \
#   bash hack/kopia-block-pvc-smoke-test.sh
#
# Remove Kubernetes resources and remote snapshots left by previous runs:
#
#   bash hack/kopia-block-pvc-smoke-test.sh cleanup
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
KOPIA_BUCKET="${KOPIA_BUCKET:-}"
KOPIA_ENDPOINT="${KOPIA_ENDPOINT:-}"
KOPIA_REGION="${KOPIA_REGION:-}"
KOPIA_VIRTUAL_HOSTED_STYLE="${KOPIA_VIRTUAL_HOSTED_STYLE:-}"
KOPIA_PREFIX="${KOPIA_PREFIX:-kopia/}"
KOPIA_DISABLE_TLS="${KOPIA_DISABLE_TLS:-auto}"
KOPIA_CONTENT_CACHE_SIZE_MB="${KOPIA_CONTENT_CACHE_SIZE_MB:-256}"
KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB="${KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB:-512}"
KOPIA_METADATA_CACHE_SIZE_MB="${KOPIA_METADATA_CACHE_SIZE_MB:-256}"
KOPIA_METADATA_CACHE_SIZE_LIMIT_MB="${KOPIA_METADATA_CACHE_SIZE_LIMIT_MB:-512}"
PVC_SIZE="${PVC_SIZE:-1Gi}"
PAYLOAD_MIB="${PAYLOAD_MIB:-64}"
STORAGE_CLASS="${STORAGE_CLASS:-}"
TIMEOUT="${TIMEOUT:-30m}"
KEEP="${KEEP:-false}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)}"

JOB_TTL_LINE=""
if [[ "${KEEP}" != "true" ]]; then
	JOB_TTL_LINE="  ttlSecondsAfterFinished: 300"
fi

SOURCE_PVC="${SOURCE_PVC:-kopia-source-${RUN_ID}}"
TARGET_PVC="${TARGET_PVC:-kopia-target-${RUN_ID}}"
WRITE_JOB="${WRITE_JOB:-kopia-write-${RUN_ID}}"
BACKUP_JOB="${BACKUP_JOB:-kopia-backup-${RUN_ID}}"
RESTORE_JOB="${RESTORE_JOB:-kopia-restore-${RUN_ID}}"
VMBACKUP_TAG="${VMBACKUP_TAG:-kopia-smoke-${RUN_ID}}"
SNAPSHOT_TAG="${SNAPSHOT_TAG:-${SOURCE_PVC}}"
SMOKE_TEST_LABEL="harvesterhci.io/smoke-test"
SMOKE_TEST_VALUE="kopia-block-pvc"
SMOKE_TEST_TAG="smoke:kopia-block-pvc"
CLEANUP_JOB="${CLEANUP_JOB:-kopia-cleanup-${RUN_ID}}"

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

for cache_value in \
	"${KOPIA_CONTENT_CACHE_SIZE_MB}" \
	"${KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB}" \
	"${KOPIA_METADATA_CACHE_SIZE_MB}" \
	"${KOPIA_METADATA_CACHE_SIZE_LIMIT_MB}"; do
	if [[ ! "${cache_value}" =~ ^[1-9][0-9]*$ ]]; then
		echo "Kopia cache sizes must be positive integer MiB values, got: ${cache_value}" >&2
		exit 1
	fi
done
KOPIA_CACHE_FLAGS="--content-cache-size-mb=${KOPIA_CONTENT_CACHE_SIZE_MB} --content-cache-size-limit-mb=${KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB} --metadata-cache-size-mb=${KOPIA_METADATA_CACHE_SIZE_MB} --metadata-cache-size-limit-mb=${KOPIA_METADATA_CACHE_SIZE_LIMIT_MB}"

BACKUP_TARGET_VALUE="$(kubectl get settings.harvesterhci.io "${BACKUP_TARGET_SETTING}" -o jsonpath='{.value}')"
BACKUP_TARGET_TYPE="$(jq -er '.type' <<<"${BACKUP_TARGET_VALUE}")"
if [[ "${BACKUP_TARGET_TYPE}" != "s3" ]]; then
	echo "Kopia smoke test requires an S3 backup target, got: ${BACKUP_TARGET_TYPE}" >&2
	exit 1
fi

if [[ -z "${KOPIA_BUCKET}" ]]; then
	KOPIA_BUCKET="$(jq -er '.bucketName | select(length > 0)' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${KOPIA_ENDPOINT}" ]]; then
	KOPIA_ENDPOINT="$(jq -r '.endpoint // ""' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${KOPIA_REGION}" ]]; then
	KOPIA_REGION="$(jq -r '.bucketRegion // ""' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ -z "${KOPIA_VIRTUAL_HOSTED_STYLE}" ]]; then
	KOPIA_VIRTUAL_HOSTED_STYLE="$(jq -r '.virtualHostedStyle // false' <<<"${BACKUP_TARGET_VALUE}")"
fi
if [[ "${KOPIA_VIRTUAL_HOSTED_STYLE}" != "true" && "${KOPIA_VIRTUAL_HOSTED_STYLE}" != "false" ]]; then
	echo "KOPIA_VIRTUAL_HOSTED_STYLE must be true or false" >&2
	exit 1
fi
while [[ "${KOPIA_ENDPOINT}" == */ ]]; do
	KOPIA_ENDPOINT="${KOPIA_ENDPOINT%/}"
done
REPOSITORY_ID="sha256:$(
	printf '%s\0%s\0%s\0%s\0%s' \
		"${BACKUP_TARGET_TYPE}" \
		"${KOPIA_ENDPOINT}" \
		"${KOPIA_BUCKET}" \
		"${KOPIA_REGION}" \
		"${KOPIA_VIRTUAL_HOSTED_STYLE}" | sha256_hex
)"
KOPIA_REPOSITORY_PASSWORD="$(
	printf '%s\0%s' "kopia" "${REPOSITORY_ID}" | sha256_hex
)"
if [[ -z "${KOPIA_ENDPOINT}" ]]; then
	KOPIA_ENDPOINT="s3.amazonaws.com"
fi

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
						(.metadata.name | test("^kopia-(write|backup|restore|cleanup)-"))
					 else
						(.metadata.name | test("^kopia-(source|target)-"))
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
	echo
	echo "Job and pod termination status:"
	kubectl -n "${NAMESPACE}" get "job/${job}" -o json | jq '{status: .status}' || true
	kubectl -n "${NAMESPACE}" get pods -l "job-name=${job}" -o json | jq '
		.items[] | {
			pod: .metadata.name,
			phase: .status.phase,
			reason: .status.reason,
			message: .status.message,
			containers: [.status.containerStatuses[]? | {
				name,
				state,
				lastState
			}]
		}' || true
	echo "Pod events:"
	kubectl -n "${NAMESPACE}" describe pods -l "job-name=${job}" | sed -n '/Events:/,$p' || true
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
        resources:
          requests:
            ephemeral-storage: 512Mi
          limits:
            ephemeral-storage: 3Gi
        env:
        - name: KOPIA_CACHE_DIRECTORY
          value: /kopia-cache
        - name: KOPIA_LOG_DIR
          value: /kopia-cache
        - name: KOPIA_BUCKET
          value: "${KOPIA_BUCKET}"
        - name: KOPIA_DISABLE_TLS
          value: "${KOPIA_DISABLE_TLS}"
        - name: KOPIA_ENDPOINT
          value: "${KOPIA_ENDPOINT}"
        - name: KOPIA_REGION
          value: "${KOPIA_REGION}"
        - name: KOPIA_PREFIX
          value: "${KOPIA_PREFIX}"
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
        - name: KOPIA_PASSWORD
          value: "${KOPIA_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          ENDPOINT="\${KOPIA_ENDPOINT#http://}"
          ENDPOINT="\${ENDPOINT#https://}"
          ENDPOINT="\${ENDPOINT%/}"
          DISABLE_TLS=""
          if [ "\${KOPIA_DISABLE_TLS:-}" = "true" ] || [ "\${KOPIA_ENDPOINT#http://}" != "\${KOPIA_ENDPOINT}" ]; then
            DISABLE_TLS="--disable-tls"
          fi
          kopia --config-file=/tmp/kopia.config repository connect s3 \${DISABLE_TLS} --cache-directory=/kopia-cache ${KOPIA_CACHE_FLAGS} --bucket="\${KOPIA_BUCKET}" --endpoint="\${ENDPOINT}" --region="\${KOPIA_REGION}" --prefix="\${KOPIA_PREFIX}" --access-key="\${AWS_ACCESS_KEY_ID}" --secret-access-key="\${AWS_SECRET_ACCESS_KEY}"
          SNAPSHOT_JSON=\$(kopia --config-file=/tmp/kopia.config snapshot list --all --json --tags 'ns:${NAMESPACE}')
          IDS=\$(printf '%s' "\${SNAPSHOT_JSON}" | jq -r '
            .[] |
            select(
              (.tags["tag:smoke"] // "") == "kopia-block-pvc" or
              (.tags["tag:vmb"] // "") == "${VMBACKUP_TAG}" or
              ((.tags["tag:vmb"] // "") | startswith("kopia-smoke-"))
            ) |
            .id
          ')
          if [ -n "\${IDS}" ]; then
            # Snapshot IDs contain no whitespace, so intentional word splitting is safe here.
            kopia --config-file=/tmp/kopia.config snapshot delete --delete \${IDS}
          else
            echo 'no matching Kopia smoke-test snapshots found'
          fi
        volumeMounts:
        - name: kopia-cache
          mountPath: /kopia-cache
      volumes:
      - name: kopia-cache
        emptyDir:
          sizeLimit: 2Gi
EOF

	if ! wait_job "${CLEANUP_JOB}"; then
		remote_cleanup_status=1
	fi
	kubectl -n "${NAMESPACE}" delete job "${CLEANUP_JOB}" --ignore-not-found
	if ((remote_cleanup_status != 0)); then
		echo "remote Kopia snapshot cleanup failed" >&2
		return "${remote_cleanup_status}"
	fi
	echo "removed Kopia smoke-test snapshots and Kubernetes resources from ${NAMESPACE}"
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
${JOB_TTL_LINE}
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
${JOB_TTL_LINE}
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
        - name: KOPIA_CACHE_DIRECTORY
          value: /kopia-cache
        - name: KOPIA_LOG_DIR
          value: /kopia-cache
        - name: KOPIA_BUCKET
          value: "${KOPIA_BUCKET}"
        - name: KOPIA_DISABLE_TLS
          value: "${KOPIA_DISABLE_TLS}"
        - name: KOPIA_ENDPOINT
          value: "${KOPIA_ENDPOINT}"
        - name: KOPIA_REGION
          value: "${KOPIA_REGION}"
        - name: KOPIA_PREFIX
          value: "${KOPIA_PREFIX}"
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
        - name: KOPIA_PASSWORD
          value: "${KOPIA_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          ENDPOINT="\${KOPIA_ENDPOINT#http://}"
          ENDPOINT="\${ENDPOINT#https://}"
          ENDPOINT="\${ENDPOINT%/}"
          DISABLE_TLS=""
          if [ "\${KOPIA_DISABLE_TLS:-}" = "true" ] || [ "\${KOPIA_ENDPOINT#http://}" != "\${KOPIA_ENDPOINT}" ]; then
            DISABLE_TLS="--disable-tls"
          fi
          kopia --config-file=/tmp/kopia.config repository connect s3 \${DISABLE_TLS} --cache-directory=/kopia-cache ${KOPIA_CACHE_FLAGS} --bucket="\${KOPIA_BUCKET}" --endpoint="\${ENDPOINT}" --region="\${KOPIA_REGION}" --prefix="\${KOPIA_PREFIX}" --access-key="\${AWS_ACCESS_KEY_ID}" --secret-access-key="\${AWS_SECRET_ACCESS_KEY}" || \
            { kopia --config-file=/tmp/kopia.config repository create s3 \${DISABLE_TLS} --cache-directory=/kopia-cache ${KOPIA_CACHE_FLAGS} --bucket="\${KOPIA_BUCKET}" --endpoint="\${ENDPOINT}" --region="\${KOPIA_REGION}" --prefix="\${KOPIA_PREFIX}" --access-key="\${AWS_ACCESS_KEY_ID}" --secret-access-key="\${AWS_SECRET_ACCESS_KEY}" || \
              kopia --config-file=/tmp/kopia.config repository connect s3 \${DISABLE_TLS} --cache-directory=/kopia-cache ${KOPIA_CACHE_FLAGS} --bucket="\${KOPIA_BUCKET}" --endpoint="\${ENDPOINT}" --region="\${KOPIA_REGION}" --prefix="\${KOPIA_PREFIX}" --access-key="\${AWS_ACCESS_KEY_ID}" --secret-access-key="\${AWS_SECRET_ACCESS_KEY}"; }
          /usr/bin/harvester io-mode -device /dev/source -mode=read | \
            kopia --config-file=/tmp/kopia.config snapshot create /${SOURCE_PV} --stdin-file ${SOURCE_PV} --tags ns:${NAMESPACE} --tags vmb:${VMBACKUP_TAG} --tags sn:${SNAPSHOT_TAG} --tags ${SMOKE_TEST_TAG}
        volumeMounts:
        - name: kopia-cache
          mountPath: /kopia-cache
        volumeDevices:
        - name: source
          devicePath: /dev/source
      volumes:
      - name: kopia-cache
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
${JOB_TTL_LINE}
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
        - name: KOPIA_CACHE_DIRECTORY
          value: /kopia-cache
        - name: KOPIA_LOG_DIR
          value: /kopia-cache
        - name: KOPIA_BUCKET
          value: "${KOPIA_BUCKET}"
        - name: KOPIA_DISABLE_TLS
          value: "${KOPIA_DISABLE_TLS}"
        - name: KOPIA_ENDPOINT
          value: "${KOPIA_ENDPOINT}"
        - name: KOPIA_REGION
          value: "${KOPIA_REGION}"
        - name: KOPIA_PREFIX
          value: "${KOPIA_PREFIX}"
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
        - name: KOPIA_PASSWORD
          value: "${KOPIA_REPOSITORY_PASSWORD}"
        args:
        - |
          set -eo pipefail
          ENDPOINT="\${KOPIA_ENDPOINT#http://}"
          ENDPOINT="\${ENDPOINT#https://}"
          ENDPOINT="\${ENDPOINT%/}"
          DISABLE_TLS=""
          if [ "\${KOPIA_DISABLE_TLS:-}" = "true" ] || [ "\${KOPIA_ENDPOINT#http://}" != "\${KOPIA_ENDPOINT}" ]; then
            DISABLE_TLS="--disable-tls"
          fi
          kopia --config-file=/tmp/kopia.config repository connect s3 \${DISABLE_TLS} --cache-directory=/kopia-cache ${KOPIA_CACHE_FLAGS} --bucket="\${KOPIA_BUCKET}" --endpoint="\${ENDPOINT}" --region="\${KOPIA_REGION}" --prefix="\${KOPIA_PREFIX}" --access-key="\${AWS_ACCESS_KEY_ID}" --secret-access-key="\${AWS_SECRET_ACCESS_KEY}"
          SNAPSHOT_JSON=\$(kopia --config-file=/tmp/kopia.config snapshot list --all --json --tags ns:${NAMESPACE} --tags vmb:${VMBACKUP_TAG} --tags sn:${SNAPSHOT_TAG})
          OBJ=\$(printf '%s' "\${SNAPSHOT_JSON}" | jq -er 'last | .rootEntry.obj') || { echo 'no matching Kopia snapshot found' >&2; exit 1; }
          kopia --config-file=/tmp/kopia.config show "\${OBJ}/${SOURCE_PV}" | /usr/bin/harvester io-mode -device /dev/target -mode=write
          sync
          HASH=\$(sha256sum /dev/target)
          HASH=\${HASH%% *}
          echo "TARGET_SHA256 \${HASH}"
          if [ "\${HASH}" != "${SOURCE_HASH}" ]; then
            echo "data integrity check failed: source=${SOURCE_HASH} target=\${HASH}" >&2
            exit 1
          fi
          if [ "${KEEP}" != "true" ]; then
            IDS=\$(printf '%s' "\${SNAPSHOT_JSON}" | jq -r '.[].id')
            if [ -n "\${IDS}" ]; then
              kopia --config-file=/tmp/kopia.config snapshot delete --delete \${IDS}
            fi
          fi
        volumeMounts:
        - name: kopia-cache
          mountPath: /kopia-cache
        volumeDevices:
        - name: target
          devicePath: /dev/target
      volumes:
      - name: kopia-cache
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

echo "kopia block PVC smoke test passed"
echo "namespace=${NAMESPACE}"
echo "source_pvc=${SOURCE_PVC}"
echo "source_pv=${SOURCE_PV}"
echo "target_pvc=${TARGET_PVC}"
echo "repository=${KOPIA_ENDPOINT}/${KOPIA_BUCKET}/${KOPIA_PREFIX} region=${KOPIA_REGION:-<default>}"
echo "tags=ns:${NAMESPACE},vmb:${VMBACKUP_TAG},sn:${SNAPSHOT_TAG}"
