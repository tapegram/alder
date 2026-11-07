#!/bin/sh
# The Script target of `alder deploy local` (book chapter 16). The command runs this from the
# app's root after the gate passed, with two arguments: the artifact directory (absolute:
# build/<app>/deploy/<build id>/, holding the executable `app`, static/, migrations/ and
# alder-deploy.json) and the environment's name; ALDER_REQUIRES holds the names of the variables
# the target must provide, comma-separated. Its exit code is the deploy's result. This one only
# records what it was given, in a file next to the artifact, so the example deploys with nothing
# installed.
set -eu
artifact="$1"
env="$2"
{
  echo "args: $*"
  echo "env: $env"
  echo "build: $(basename "$artifact")"
  echo "ALDER_REQUIRES: ${ALDER_REQUIRES-}"
} > "$artifact.deploy-local.txt"
