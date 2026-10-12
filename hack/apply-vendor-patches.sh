#!/usr/bin/env bash
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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
#
# Re-applies local patches on top of `go mod vendor` output.
# Idempotent: patches that are already applied are skipped.

set -euo pipefail

PATCHES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/patches"

for patch_file in "${PATCHES_DIR}"/*.patch; do
    [ -e "${patch_file}" ] || exit 0
    if git apply --reverse --check "${patch_file}" >/dev/null 2>&1; then
        echo "already applied: ${patch_file}"
    else
        echo "applying: ${patch_file}"
        git apply "${patch_file}"
    fi
done
