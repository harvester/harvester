#!/bin/bash -ex

TARGET_FILES=$(ls)
echo "Cleanup the test files, whole files: ${TARGET_FILES}"

rm -rf .terraform/ || true
rm -rf .terraform.lock.hcl || true
rm -rf ./kubeconf || true
rm -f ./terraform_bin.zip || true
rm -f ./terraform || true