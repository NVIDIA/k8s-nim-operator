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
# Writes THIRD_PARTY_NOTICES.md for the two Go binaries the released image ships.

set -euo pipefail

OUTPUT="${OUTPUT:-THIRD_PARTY_NOTICES.md}"
LICENSES_DIR="${LICENSES_DIR:-.licenses-cache}"
MULTI_ARCH_MK="${MULTI_ARCH_MK:-deployments/container/multi-arch.mk}"
MODULES_TXT="${MODULES_TXT:-vendor/modules.txt}"
DOCKERFILE="${DOCKERFILE:-deployments/container/Dockerfile}"

# Exactly what the Dockerfile compiles; the e2e suite under test/ never ships.
PACKAGES=("./cmd/...")

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
)

die() {
    printf 'ERROR: %s\n' "$1" >&2
    shift
    if (( $# > 0 )); then
        printf '%s\n' "$@" >&2
    fi
    exit 1
}

log() {
    printf '%s\n' "$*" >&2
}

# go-licenses tests stdlib membership against the GOROOT compiled into its own
# binary, so another toolchain makes stdlib look module-less. Call after any cd.
export_matching_goroot() {
    GOROOT="$(go env GOROOT)"
    export GOROOT
}

# Licenses that are themselves Markdown close a fixed ``` fence early, so open
# with one backtick more than the file's longest run. -a: a NUL byte otherwise
# makes grep report "Binary file ... matches" instead of the matches.
fence_for() {
    local file="$1" longest width
    longest=$(LC_ALL=C grep -oaE '`+' "${file}" 2>/dev/null \
        | awk '{ if (length($0) > m) m = length($0) } END { print m+0 }')
    width=$(( longest + 1 ))
    (( width < 3 )) && width=3
    printf '%*s' "${width}" '' | tr ' ' '`'
}

check_prerequisites() {
    command -v go >/dev/null 2>&1 || die "go is not installed."
    command -v git >/dev/null 2>&1 || die "git is not installed."

    # Probe by running it: a foreign-OS binary passes -x but cannot exec.
    if ./bin/go-licenses --help >/dev/null 2>&1; then
        GO_LICENSES="${PWD}/bin/go-licenses"
    elif command -v go-licenses >/dev/null 2>&1; then
        GO_LICENSES="$(command -v go-licenses)"
    else
        die "go-licenses is missing or was built for another platform." \
            "If ./bin/go-licenses exists, it cannot run here: remove it and re-run" \
            "'make third-party-notices', which rebuilds it for this host."
    fi

    local f
    for f in "${MULTI_ARCH_MK}" "${MODULES_TXT}" "${DOCKERFILE}"; do
        [[ -f "${f}" ]] || die "${f} not found — run 'make third-party-notices' from the repo root."
    done

    LOCAL_MODULE=$(go list -m 2>/dev/null || true)
    [[ -n "${LOCAL_MODULE}" ]] || die "could not determine local module path via 'go list -m'."

    # CGO off matches the Dockerfile and lets go-licenses cross-list without a C
    # toolchain; the wrong setting can drop whole packages while still exiting 0.
    export GOFLAGS="-mod=vendor"
    export CGO_ENABLED=0
    export_matching_goroot
}

read_helper_binary() {
    HELPER_REPO_URL=$(sed -n \
        's/^RUN[[:space:]]\{1,\}git clone[[:space:]]\{1,\}\([^[:space:]]\{1,\}\).*/\1/p' \
        "${DOCKERFILE}" | head -n 1)
    [[ -n "${HELPER_REPO_URL}" ]] || die \
        "could not find a 'RUN git clone <url>' line in ${DOCKERFILE}." \
        "hack/generate-third-party-notices.sh reads the helper binary's source from there;" \
        "teach it the new form rather than dropping the binary from the notices."

    HELPER_COMMIT=$(sed -n \
        's/^RUN[[:space:]]\{1,\}git checkout[[:space:]]\{1,\}\([0-9a-fA-F]\{40\}\).*/\1/p' \
        "${DOCKERFILE}" | head -n 1)
    [[ -n "${HELPER_COMMIT}" ]] || die \
        "could not find a 'RUN git checkout <40-hex commit>' line in ${DOCKERFILE}." \
        "A branch or tag would not pin the dependency set, so refusing to guess."

    local build_line
    build_line=$(LC_ALL=C grep -m 1 -E '^RUN[[:space:]]+go build[[:space:]]+-o[[:space:]]' \
        "${DOCKERFILE}" || true)
    [[ -n "${build_line}" ]] || die \
        "could not find a 'RUN go build -o <binary> <path>' line in ${DOCKERFILE}."

    HELPER_BINARY=$(printf '%s\n' "${build_line}" | awk '{ for (i = 1; i < NF; i++) if ($i == "-o") { print $(i + 1); exit } }')
    local build_src
    build_src=$(printf '%s\n' "${build_line}" | awk '{ print $NF }')
    [[ -n "${HELPER_BINARY}" && -n "${build_src}" ]] || die \
        "could not read the binary name and source path from: ${build_line}"

    # A .go file loads as "command-line-arguments" and carries no module info.
    case "${build_src}" in
        *.go) HELPER_PACKAGE="./$(dirname "${build_src}")" ;;
        *)    HELPER_PACKAGE="./${build_src#./}" ;;
    esac
}

verify_platform_matrix() {
    local expected actual
    expected=$(sed -n 's/^DOCKER_BUILD_PLATFORM_OPTIONS[[:space:]]*?*=[[:space:]]*--platform=//p' \
        "${MULTI_ARCH_MK}" | tr ',' '\n' | sed '/^$/d' | LC_ALL=C sort -u)
    [[ -n "${expected}" ]] \
        || die "could not read DOCKER_BUILD_PLATFORM_OPTIONS from ${MULTI_ARCH_MK}."

    actual=$(printf '%s\n' "${PLATFORMS[@]}" | LC_ALL=C sort -u)
    [[ "${expected}" == "${actual}" ]] || die \
        "the PLATFORMS matrix is out of sync with ${MULTI_ARCH_MK}." \
        "Update the PLATFORMS array in hack/generate-third-party-notices.sh to match the released targets." \
        "  matrix (PLATFORMS): $(echo "${actual}" | paste -sd ' ' -)" \
        "  image platforms:    $(echo "${expected}" | paste -sd ' ' -)"
}

prepare_workspace() {
    # Guard the override: '', '/', '.' or '..' would make this 'rm -rf' fatal.
    case "${LICENSES_DIR}" in
        ""|"/"|"."|"..")
            die "refusing to 'rm -rf' unsafe LICENSES_DIR='${LICENSES_DIR}'."
            ;;
    esac
    rm -rf "${LICENSES_DIR}"
    mkdir -p "${LICENSES_DIR}" "${LICENSES_DIR}/.helper"

    # Explicit templates: macOS mktemp ignores TMPDIR without one.
    local t="${TMPDIR:-/tmp}/k8s-nim-operator-notices"
    SAVE_ROOT="$(mktemp -d "${t}.XXXXXX")"
    COMBINED_CSV="$(mktemp "${t}-csv.XXXXXX")"
    INDEX_FILE="$(mktemp "${t}-idx.XXXXXX")"
    HELPER_CSV="$(mktemp "${t}-helper-csv.XXXXXX")"
    HELPER_INDEX="$(mktemp "${t}-helper-idx.XXXXXX")"
    # Composed beside its destination, not under TMPDIR, so publishing it is a
    # rename(2) within one filesystem rather than a copy-then-unlink.
    mkdir -p "$(dirname "${OUTPUT}")"
    OUT_TMP="$(mktemp "${OUTPUT}.XXXXXX")"
    HELPER_SRC="${SAVE_ROOT}/helper-src"
    trap 'rm -rf "${SAVE_ROOT}"; rm -f "${COMBINED_CSV}" "${INDEX_FILE}" "${HELPER_CSV}" "${HELPER_INDEX}" "${OUT_TMP}"' EXIT
}

collect_manager() {
    local platform goos goarch save_dir

    for platform in "${PLATFORMS[@]}"; do
        goos="${platform%/*}"
        goarch="${platform#*/}"
        log "Collecting licenses for ${goos}/${goarch}..."

        save_dir="${SAVE_ROOT}/${goos}_${goarch}"

        # Only the local module: --ignore matches raw string prefixes, not path
        # segments, so "go" would drop golang.org/*, google.golang.org/*, gopkg.in/*.
        GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" save "${PACKAGES[@]}" \
            --save_path="${save_dir}" \
            --force \
            --ignore="${LOCAL_MODULE}"

        GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" csv "${PACKAGES[@]}" \
            --ignore="${LOCAL_MODULE}" \
            >> "${COMBINED_CSV}"

        merge_licenses "${save_dir}" "${LICENSES_DIR}"
    done
}

collect_helper_binary() {
    local platform goos goarch save_dir

    log "Cloning ${HELPER_REPO_URL} at ${HELPER_COMMIT}..."
    mkdir -p "${HELPER_SRC}"
    (
        cd "${HELPER_SRC}"
        git init -q .
        git remote add origin "${HELPER_REPO_URL}"
        git fetch -q --depth 1 origin "${HELPER_COMMIT}"
        git checkout -q FETCH_HEAD
    ) || die \
        "could not fetch ${HELPER_COMMIT} from ${HELPER_REPO_URL}." \
        "This pass needs network access to the host serving that repository."

    HELPER_MODULE=$( cd "${HELPER_SRC}" && GOFLAGS="-mod=readonly" go list -m 2>/dev/null || true )
    [[ -n "${HELPER_MODULE}" ]] \
        || die "could not determine the module path of the cloned ${HELPER_REPO_URL}."

    ( cd "${HELPER_SRC}" && GOFLAGS="-mod=readonly" go mod download ) >&2

    for platform in "${PLATFORMS[@]}"; do
        goos="${platform%/*}"
        goarch="${platform#*/}"
        log "Collecting ${HELPER_BINARY} licenses for ${goos}/${goarch}..."

        save_dir="${SAVE_ROOT}/helper/${goos}_${goarch}"
        (
            cd "${HELPER_SRC}"
            # shellcheck disable=SC2030  # subshell-local: outer -mod=vendor stands.
            export GOFLAGS="-mod=readonly"
            export_matching_goroot
            GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" save "${HELPER_PACKAGE}" \
                --save_path="${save_dir}" \
                --force \
                --ignore="${LOCAL_MODULE}" >&2
            GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" csv "${HELPER_PACKAGE}" \
                --ignore="${LOCAL_MODULE}"
        ) >> "${HELPER_CSV}"

        # Separate subtree: the manager graph has these paths at other versions.
        merge_licenses "${save_dir}" "${LICENSES_DIR}/.helper"
    done
}

# Module cache files are 0444 and cp preserves that, so restore write permission
# or the next platform's copy fails.
merge_licenses() {
    cp -R "$1/." "$2/"
    chmod -R u+w "$2"
}

# Join licenses rather than pick one: go-licenses emits a row per recognized
# license, so key-only dedup hides the second and differs between sorts.
collapse_index() {
    LC_ALL=C sort -u "$1" | awk -F, '
        {
            pkg = $1
            if (!(pkg in url)) { url[pkg] = $2; order[++n] = pkg }
            if (!((pkg SUBSEP $3) in seen)) {
                seen[pkg SUBSEP $3] = 1
                # Count, do not test "pkg in lic": mawk instantiates the
                # assignment target first, so every license would get a " / ".
                lic[pkg] = (cnt[pkg]++ ? lic[pkg] " / " : "") $3
            }
        }
        END { for (i = 1; i <= n; i++) print order[i] "," url[order[i]] "," lic[order[i]] }
    '
}

# Rows carry the module path, not a URL: in vendor mode go-licenses links this
# repo at HEAD. Longest-prefix match — a license may sit below the module root.
annotate_modules() {
    awk -v modfile="${MODULES_TXT}" '
        BEGIN {
            FS = OFS = ","
            while ((getline line < modfile) > 0) {
                if (line !~ /^# /) continue
                split(line, f, " ")
                # Report the replacement; a local path cannot be attributed.
                if (f[4] == "=>" || f[3] == "=>") {
                    r = (f[4] == "=>") ? 5 : 4
                    if (f[r + 1] == "") {
                        print "ERROR: " modfile " replaces " f[2] " with a local path;" > "/dev/stderr"
                        print "teach hack/generate-third-party-notices.sh how to attribute it." > "/dev/stderr"
                        exit 1
                    }
                    mods[++m] = f[2]
                    disp[f[2]] = f[r]
                } else {
                    mods[++m] = f[2]
                    disp[f[2]] = f[2]
                }
            }
            close(modfile)
            # A read error returns -1, which would label everything "unknown".
            if (m == 0) {
                print "ERROR: no module lines read from " modfile > "/dev/stderr"
                exit 1
            }
        }
        {
            best = ""
            for (i = 1; i <= m; i++) {
                mp = mods[i]
                if (($1 == mp || index($1, mp "/") == 1) && length(mp) > length(best)) best = mp
            }
            print $0, (best == "" ? "unknown" : disp[best])
        }
    '
}

# The clone is its own main module, so go-licenses only offers a HEAD URL.
pin_helper_urls() {
    awk -v mod="${HELPER_MODULE}" -v commit="${HELPER_COMMIT}" '
        BEGIN { FS = OFS = "," }
        {
            if ($1 == mod || index($1, mod "/") == 1) {
                sub(/\/blob\/HEAD\//, "/blob/" commit "/", $2)
            }
            print
        }
    '
}

build_indexes() {
    log "Generating dependency index..."
    collapse_index "${COMBINED_CSV}" | annotate_modules > "${INDEX_FILE}"
    collapse_index "${HELPER_CSV}" | pin_helper_urls > "${HELPER_INDEX}"

    [[ -s "${INDEX_FILE}" ]] \
        || die "go-licenses produced no entries for ${PACKAGES[*]} — refusing to write empty notices file."
    [[ -s "${HELPER_INDEX}" ]] \
        || die "go-licenses produced no entries for ${HELPER_BINARY} — refusing to write incomplete notices file."

    if cut -d, -f4 "${INDEX_FILE}" | LC_ALL=C grep -qx 'unknown'; then
        die "could not resolve modules for some manager packages from ${MODULES_TXT}." \
            "Run 'go mod vendor' and re-run, rather than committing a file with unattributed entries."
    fi

    # An empty field would also render as "Unknown" via the :- fallback in the
    # table, so catch it here rather than letting it reach the document. Both
    # shipped surfaces are checked: an unclassifiable license in either one
    # would otherwise attribute nothing.
    local idx
    for idx in "${INDEX_FILE}" "${HELPER_INDEX}"; do
        if cut -d, -f3 "${idx}" | LC_ALL=C grep -qE '^$|(^| / )Unknown( / |$)'; then
            die "go-licenses could not identify a license for some dependencies." \
                "Check the entries reported as Unknown before committing the file."
        fi
    done

    # go-licenses falls back to "Unknown" with a zero exit when a lookup fails.
    if cut -d, -f2 "${HELPER_INDEX}" | LC_ALL=C grep -qx 'Unknown'; then
        die "go-licenses could not resolve source URLs for some ${HELPER_BINARY} modules." \
            "This usually means the network blocked a '?go-get=1' lookup. Re-run with" \
            "access to the module hosts rather than committing a degraded file."
    fi

    if LC_ALL=C grep -q '/blob/HEAD/' "${HELPER_INDEX}"; then
        die "some ${HELPER_BINARY} entries still reference a HEAD URL rather than a pinned revision." \
            "Those links stop describing the built content once the branch moves;" \
            "teach hack/generate-third-party-notices.sh how to pin them."
    fi
}

# Filter by name: for restricted licenses 'go-licenses save' copies the source.
license_files_for() {
    local dir="$1" f
    [[ -d "${dir}" ]] || return 0
    while IFS= read -r -d '' f; do
        # LC_ALL=C, as on every sort and grep here: under a Turkish locale glibc
        # does not fold I to i, so this would stop matching LICENSE.
        if printf '%s' "$(basename "${f}")" \
            | LC_ALL=C grep -qiE '^(licen[cs]e|notice|copying|copyright|authors|patents)([-._].*)?$'; then
            printf '%s\n' "${f}"
        fi
    done < <(find "${dir}" -maxdepth 1 -type f -print0 2>/dev/null | LC_ALL=C sort -z)
}

# ${2}: "module" for vendored rows, "source" for cloned ones.
emit_index_table() {
    local index="$1" provenance="$2" pkg url license module
    if [[ "${provenance}" == "module" ]]; then
        printf '| Package | License | Dependency |\n'
    else
        printf '| Package | License | Source |\n'
    fi
    printf '|---------|---------|--------|\n'

    while IFS=, read -r pkg url license module; do
        [[ -z "${pkg}" ]] && continue
        # shellcheck disable=SC2016  # backticks are literal markdown here.
        if [[ "${provenance}" == "module" ]]; then
            printf '| `%s` | %s | `%s` |\n' "${pkg}" "${license:-Unknown}" "${module:-unknown}"
        else
            printf '| `%s` | %s | %s |\n' "${pkg}" "${license:-Unknown}" "${url:-n/a}"
        fi
    done < "${index}"
}

emit_sections() {
    local index="$1" root="$2" provenance="$3"
    local pkg url license module files lf fence

    while IFS=, read -r pkg url license module; do
        [[ -z "${pkg}" ]] && continue

        printf '### %s\n\n' "${pkg}"
        printf '* License: %s\n' "${license:-Unknown}"
        if [[ "${provenance}" == "module" ]]; then
            printf '* Module: %s\n\n' "${module:-unknown}"
        else
            printf '* Source: %s\n\n' "${url:-n/a}"
        fi

        files=()
        while IFS= read -r lf; do
            [[ -n "${lf}" ]] && files+=("${lf}")
        done < <(license_files_for "${root}/${pkg}")

        if (( ${#files[@]} == 0 )); then
            printf 'License text unavailable. See upstream source for the full license.\n'
        else
            for lf in "${files[@]}"; do
                fence="$(fence_for "${lf}")"
                printf '#### %s\n\n' "$(basename "${lf}")"
                printf '%stext\n' "${fence}"
                cat "${lf}"
                echo
                printf '%s\n' "${fence}"
                echo
            done
        fi
        echo
    done < "${index}"
}

# printf, not a quoted heredoc, so Dockerfile values reach the prose.
# shellcheck disable=SC2016  # backticks in these formats are literal markdown.
compose_document() {
    log "Composing ${OUTPUT}..."
    {
        cat <<'EOF'
# Third-Party Notices

NVIDIA NIM Operator

EOF
        printf 'This file lists the third-party **Go modules** that NVIDIA NIM Operator\n'
        printf 'redistributes, along with the verbatim text of each license. The released\n'
        printf 'image ships two Go binaries: `manager`, the operator itself, built from\n'
        printf '`cmd/` and run as the image entrypoint; and `%s`, built from\n' "${HELPER_BINARY}"
        printf '`%s` at commit\n' "${HELPER_MODULE}"
        printf '`%s` and copied to `/usr/local/bin`.\n' "${HELPER_COMMIT}"
        printf 'Both are resolved as the union across every released image platform, and\n'
        printf 'the two resolve the Kubernetes client libraries to different versions, so\n'
        printf 'each is listed in its own table. Go standard library packages are excluded;\n'
        printf 'they are covered by the license of the Go distribution itself.\n\n'
        cat <<'EOF'
The image uses `nvcr.io/nvidia/distroless/go` as a base image. All of the OSS
packages and source included in this image can be found at
<https://developer.nvidia.com/w/distroless-oss/index.html>.

## manager Dependency Index

EOF
        emit_index_table "${INDEX_FILE}" module

        printf '\n## %s Dependency Index\n\n' "${HELPER_BINARY}"
        emit_index_table "${HELPER_INDEX}" source

        cat <<'EOF'

## manager License Texts

EOF
        emit_sections "${INDEX_FILE}" "${LICENSES_DIR}" module

        printf '## %s License Texts\n\n' "${HELPER_BINARY}"
        emit_sections "${HELPER_INDEX}" "${LICENSES_DIR}/.helper" source
    } > "${OUT_TMP}"
    chmod 644 "${OUT_TMP}"
    mv "${OUT_TMP}" "${OUTPUT}"
}

main() {
    check_prerequisites
    verify_platform_matrix
    read_helper_binary
    prepare_workspace

    collect_manager
    collect_helper_binary
    build_indexes
    compose_document

    local manager_count helper_count
    manager_count=$(wc -l < "${INDEX_FILE}" | tr -d ' ')
    helper_count=$(wc -l < "${HELPER_INDEX}" | tr -d ' ')
    log "Wrote ${OUTPUT} (${manager_count} manager packages," \
        "${helper_count} ${HELPER_BINARY} packages)"
}

main "$@"
