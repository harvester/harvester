#!/bin/bash -e

echo "Set multi-cluster-management feature as dynamic"
kubectl patch feature multi-cluster-management --type merge --patch  '{"status": {"dynamic": true}}'
