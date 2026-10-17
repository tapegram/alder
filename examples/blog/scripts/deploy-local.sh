#!/bin/sh
# The Script target of `alder deploy local` (book chapter 16): the command runs this with the
# environment name, the artifact directory and the build id as arguments, and the names the target
# must provide in ALDER_REQUIRES. This one only records what it was given, in a file next to the
# artifact, so the example deploys with nothing installed.
set -eu
artifact="${2:-.}"
{
  echo "args: $*"
  echo "ALDER_REQUIRES: ${ALDER_REQUIRES-}"
} > "$artifact.deploy-local.txt"
