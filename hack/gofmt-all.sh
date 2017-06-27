#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

verify=0
if [[ ${1:-} = "--verify" || ${1:-} = "-v" ]]; then
  verify=1
fi

find_files() {
  find . -not \( \( \
    -wholename './_output' \
    -o -wholename './vendor' \
  \) -prune \) -name '*.go'
}

if [[ $verify -eq 1 ]]; then
  diff=$(find_files | xargs gofmt -s -d 2>&1)
  if [[ -n "${diff}" ]]; then
    echo "gofmt -s -w $(echo "${diff}" | awk '/^diff / { print $2 }' | tr '\n' ' ')"
    exit 1
  fi
else
  find_files | xargs gofmt -s -w
fi
