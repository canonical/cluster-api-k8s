#!/bin/bash

# Description:
#   Bootstraps a Juju cluster (1 machine) and installs all tools necessary
#   to run the CAPI e2e tests on AWS, then runs the tests.
#
# Usage:
#   $ juju-create-aws-instance.sh
#
# Assumptions:
#   - These environment variables are set:
#     - AWS_B64ENCODED_CREDENTIALS

set -o nounset
set -o pipefail

DIR="$(realpath $(dirname "${0}"))"

# Bootstrap Juju
# Juju creates the instance that will host the management cluster
juju bootstrap aws/us-east-2 vimdiesel-aws --force --bootstrap-series jammy --bootstrap-constraints "arch=amd64" --model-default test-mode=true --model-default resource-tags=owner=vimdiesel --model-default automatically-retry-hooks=false --model-default 'logging-config=<root>=DEBUG' --model-default image-stream=daily --debug

juju scp -m controller "$DIR"/run-e2e-test.sh 0:/home/ubuntu/run-e2e-test.sh

#juju ssh --model controller 0 'sudo bash -s' <"$DIR"/run-e2e-test.sh
juju exec --model controller --unit controller/0 -- AWS_B64ENCODED_CREDENTIALS=${AWS_B64ENCODED_CREDENTIALS} /home/ubuntu/run-e2e-test.sh
