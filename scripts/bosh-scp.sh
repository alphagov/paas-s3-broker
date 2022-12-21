#!/bin/bash 

set -euo pipefail

function run_command() {
    local command=$1
    local message=$2

    echo -n "${message}... "

    if ! output=$(bash -c "${command}" 2>&1); then
        echo -e "failed.\n\n"
        echo "Command Failed: ${command}"
        echo ""
        echo "Output: ${output}"
        exit 1
    fi
    echo "done."
}

command -v jq >/dev/null 2>&1 || { echo >&2 "jq is required but it's not installed.  Aborting."; exit 1; }
command -v bosh >/dev/null 2>&1 || { echo >&2 "bosh is required but it's not installed.  Aborting."; exit 1; }
command -v cut >/dev/null 2>&1 || { echo >&2 "cut is required but it's not installed.  Aborting."; exit 1; }

bosh vms --json | jq -r '.Tables[0].Rows[] | select(.instance|startswith("s3_broker/")) | .instance' | while read -r instance; do
    run_command "bosh ssh ${instance} sudo monit stop s3-broker" "stopping s3-broker on ${instance}"
    run_command "bosh scp ./amd64/s3-broker ${instance}:/tmp/s3-broker" "copying s3-broker binary to tmp on ${instance}"
    run_command "bosh ssh ${instance} sudo mv /tmp/s3-broker /var/vcap/packages/s3-broker/bin/paas-s3-broker" "moving s3-broker binary from tmp to packages on ${instance}"
    run_command "bosh ssh ${instance} sudo monit start s3-broker" "starting s3-broker on ${instance}"
done

