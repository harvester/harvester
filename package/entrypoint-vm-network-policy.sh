#!/bin/bash
set -e

exec tini -- harvester-vm-network-policy "${@}"
