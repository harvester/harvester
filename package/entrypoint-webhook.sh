#!/bin/bash
set -e

exec tini -- harvester-webhook "${@}"
