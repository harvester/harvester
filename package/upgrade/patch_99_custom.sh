# patch the kube-audit policy file into 99_custom.yaml

patch_92_harvester_kube_audit_policy() {
  TARGET_FILE=$1

  # note: the script may be called multi times as the JOB retries, skip already patched
  TARGET_INDEX=$(yq e '.stages.initramfs[0].files[].path' $TARGET_FILE | grep "92-harvester-kube-audit-policy.yaml" -n | sed -e "s/:.*//") || true

  if [ -n "$TARGET_INDEX" ]; then
    echo "kube-audit policy has been patched, skip"
    PATCH_KUBE_AUDIT_RES=2
    return 0
  fi

  # add new file 92-harvester-kube-audit-policy.yaml
  yq e '.stages.initramfs[0].files += [{"path": "/etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml", "permissions": 384, "owner": 0, "group": 0, "encoding": "", "ownerstring": "" }]' $TARGET_FILE -i

  # note: any 'yq' operation to update the file will cause the file indent is changed
  TARGET_INDEX=$(yq e '.stages.initramfs[0].files[].path' $TARGET_FILE | grep "92-harvester-kube-audit-policy.yaml" -n | sed -e "s/:.*//") || true

  if [ -n "$TARGET_INDEX" ]; then
    TARGET_INDEX=$((TARGET_INDEX-1))
    echo "the file is at $TARGET_INDEX"
    yq e '.stages.initramfs[0].files['"$TARGET_INDEX"']' $TARGET_FILE
  else
    echo "can not find newly added 92-harvester-kube-audit-policy.yaml in $TARGET_FILE, CHECK"
    return 0
  fi

  LINE_NO=$(grep "path: /etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml" $TARGET_FILE -n | sed -e "s/:.*//") || true

  if [ -n "$LINE_NO" ]; then
    LINE_NO=$((LINE_NO+4)) # from 'path' to 'content'
    echo "the target line numbert is $LINE_NO"
  else
     echo "can not find the anchor keyword 92-harvester-kube-audit-policy.yaml, break"
     return 0
  fi

  # patch now
  # 'yq' can not modify JSON content field, hard coded 'sed' is an replacement
  # the target content is as following

#        - path: /etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml
#          permissions: 384
#          owner: 0
#          group: 0
#          content: |
#            apiVersion: audit.k8s.io/v1
#            kind: Policy
#            omitStages:
#              - "ResponseStarted"
#              - "ResponseComplete"
#            rules:
#              # Any include/exclude rules are added here
#
#              # A catch-all rule to log all other (create/delete/patch) requests at the Metadata level
#              - level: Metadata
#                verbs: ["create", "delete", "patch"]
#                omitStages:
#                  - "ResponseStarted"
#                  - "ResponseComplete"

  #indent 10
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ content: |"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 12
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ apiVersion: audit.k8s.io/v1"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 12
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ kind: Policy"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 12
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ omitStages:"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 14
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ - \"ResponseStarted\""
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 14
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ - \"ResponseComplete\""
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 12
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ rules:"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 14
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ # Any include/exclude rules are added here"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 14
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ # A catch-all rule to log all other (create/delete/patch) requests at the Metadata level"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 14
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ - level: Metadata"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 16
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ verbs: [\"create\", \"delete\", \"patch\"]"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 16
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ omitStages:"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 18
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ - \"ResponseStarted\""
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #indent 18
  LINE_NO=$((LINE_NO+1))
  AUDIT_POLICY_FILE_CONTENT="\ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ \ - \"ResponseComplete\""
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_CONTENT}" $TARGET_FILE

  #validate the file is effectively patched, when malformed, yq will report error
  yq e '.stages.initramfs[0].files['"$TARGET_INDEX"']' $TARGET_FILE

  PATCH_KUBE_AUDIT_RES=1
}

# add audit-policy-file param to 90-harvester-server.yaml
patch_90_harvester_server() {
  TARGET_FILE=$1

  ALREADY_SET=$(grep "audit-policy-file: /etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml" $TARGET_FILE) || true

  if [ -n "$ALREADY_SET" ]; then
    echo "audit-policy-file param has been set in $TARGET_FILE, skip"
    PATCH_SERVER_RES=2
    return 0
  fi

  TARGET_INDEX=$(yq e '.stages.initramfs[0].files[].path' $TARGET_FILE | grep "90-harvester-server.yaml" -n | sed -e "s/:.*//") || true

  if [ -n "$TARGET_INDEX" ]; then
    echo $TARGET_INDEX
    TARGET_INDEX=$((TARGET_INDEX-1))
    yq e '.stages.initramfs[0].files['"$TARGET_INDEX"']' $TARGET_FILE
  else
    echo "can not find  90-harvester-server.yaml in $TARGET_FILE, CHECK"
    return 0
  fi

  LINE_NO=$(grep "tls-san:" $TARGET_FILE -n | sed -e "s/:.*//") || true

  if [ -n "$LINE_NO" ]; then
    LINE_NO=$((LINE_NO+2))
    echo "the target line number is $LINE_NO"
  else
     echo "can not find the anchor keyword tls-san, CHECK"
     return 0
  fi

  # patch now

  #indent 12
  AUDIT_POLICY_FILE_PARAM="\ \ \ \ \ \ \ \ \ \ \ \ audit-policy-file: /etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml"
  sed -i ${LINE_NO}'i '"${AUDIT_POLICY_FILE_PARAM}" $TARGET_FILE

  #validate the file is effectively patched
  yq e '.stages.initramfs[0].files['"$TARGET_INDEX"']' $TARGET_FILE

  PATCH_SERVER_RES=1
}

patch_99_custom () {
  SRC_FILE=$1
  TEMP_FILE=$2
  PATCH_SERVER_IN_CUSTOM=$3

  PATCH_KUBE_AUDIT_RES=0 # result: 0:fail, 1:patch success, 2:already patched
  PATCH_SERVER_RES=0

  cp -f $SRC_FILE $TEMP_FILE

  patch_92_harvester_kube_audit_policy $TEMP_FILE

  if test "$PATCH_KUBE_AUDIT_RES" -eq 0; then
    echo "fail to patch of kube-audit policy file, CHECK"
    rm -f $TEMP_FILE
    return 0 # return !0 will cause upgrade always fail here, if patch really fails
  fi

  if test "$PATCH_SERVER_IN_CUSTOM" -eq 1; then
    patch_90_harvester_server $TEMP_FILE

    if test "$PATCH_SERVER_RES" -eq 0; then
      echo "fail to patch of rke2-server file, CHECK"
      rm -f $TEMP_FILE
      return 0 # return !0 will cause upgrade always fail here, if patch really fails
    fi
  fi

  if test "$PATCH_KUBE_AUDIT_RES" -eq 1 || test "$PATCH_SERVER_RES" -eq 1; then
    # write back to source file
    echo "write patched file back to source file $SRC_FILE"
    cat $TEMP_FILE > $SRC_FILE
  else
    echo "$SRC_FILE has included the patch"
  fi

  # delete tmp file
  rm -f $TEMP_FILE

  # final result, for debug
  echo "the related content in $SRC_FILE"

  if test "$PATCH_SERVER_IN_CUSTOM" -eq 1; then
    yq e '.stages.initramfs[0].files[] | select(.path== "/etc/rancher/rke2/config.yaml.d/90-harvester-server.yaml")' $SRC_FILE
  fi

  yq e '.stages.initramfs[0].files[] | select(.path== "/etc/rancher/rke2/config.yaml.d/92-harvester-kube-audit-policy.yaml")' $SRC_FILE
}

