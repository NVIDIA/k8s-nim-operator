# NIM Operator documentation

The Fern project lives at the repository root in `fern/`. All versions use
local navigation files and page directories:

| Version | Navigation | Pages | Original Sphinx source tag |
| --- | --- | --- | --- |
| 3.1.4, default | `versions/v3.1.4.yml` | `versions/v3.1.4/pages/` | `nim-operator-v3.1.4-1` |
| 3.1.3 | `versions/v3.1.3.yml` | `versions/v3.1.3/pages/` | `nim-operator-v3.1.3-1` |
| Dev | `current.yml` | `pages/` | Development on `main`, seeded from 3.1.4 |

Version 3.1.4 is the default. Dev uses the `dev` slug and beta availability.

## Local development

Use Node.js 24 and the official Fern CLI. Run these commands from the repository
root without creating local npm package files:

```sh
npx --yes --package fern-api@5.216.0 fern check
npx --yes --package fern-api@5.216.0 fern docs md check
npx --yes --package fern-api@5.216.0 fern docs dev
```

`fern.config.json` selects CLI version `5.216.0`. The commands above run Fern's
native configuration checks, MDX checks, and local preview. No repository npm
dependencies, custom scripts, or preprocessing are required.

The preview normally runs at `http://localhost:3000/nim-operator`.
Fetching `global-theme: nvidia` requires a Fern login with NVIDIA organization
access. The shared theme supplies NVIDIA's branding.

## Authoring

Edit development pages in `fern/pages/` and maintain their navigation in
`fern/current.yml`. CI builds these tracked files after merges to `main`;
`current.yml` is not generated from another repository. After publishing, Dev
is available at `https://docs.nvidia.com/nim-operator/dev/`.

Edit released pages under `versions/v<version>/pages/` and their navigation in
`versions/v<version>.yml`. Preserve slugs and anchor aliases to keep links working.
Versions are selected by `path:` entries in `docs.yml`.

Write the NIM Operator release as `${NIM_OPERATOR_VERSION}`, such as
`--version=${NIM_OPERATOR_VERSION}`. Fern fills the placeholder from the
profile that each version selects in `docs.yml`:

| Version | Profile | `NIM_OPERATOR_VERSION` |
| --- | --- | --- |
| 3.1.4 | `v3.1.4` | `3.1.4` |
| 3.1.3 | `v3.1.3` | `3.1.3` |
| Dev | `dev` | `3.1.4`, the latest release |

To publish a release, copy `pages/` to `versions/v<version>/pages/`, add a
profile with the new value, and set `profile:` on the new version entry. Update
the `dev` profile to the new release. The copied pages need no edits.

Keep literal versions in release notes and in statements about a specific
release, such as the release that first supports NIM for LLMs 3.0.
A placeholder whose name is not in the profile stays literal, so shell
expressions such as `${NEMO_VERSION}` and `${host_folder}` remain as written.
Do not add profile variables with the same names as shell variables in code
blocks. To show a literal profile placeholder, write `\$\{NAME\}`.

## Manifest inputs and example output

Include standalone YAML using Fern's native `Code` component. For example, from
a page under `fern/pages/`:

```mdx
<Code src="../../manifests/docs/current/input/nim/serving/standalone/basic/retriever-embedding.yaml" language="yaml" />
```

All doc versions, including Dev, 3.1.3, and 3.1.4, share the sources under
`manifests/docs/current/`:

- `input/` contains the 42 standalone YAML examples, seeded from Dev.
- `output/` contains all nine original `docs/manifests/output/` files from
  `nim-operator-v3.1.4-1`. Four files supply eight output excerpts per docset;
  the other five are retained as source examples.

Documentation-specific YAML files live directly in `input/`; NIM examples
retain their `nim/` subdirectories. Output excerpts also use native `Code`
includes, with their original content, line selections, and highlights.

Edit and test the YAML files directly in the relevant Kubernetes or Helm workflow.
Changes to shared input and output files apply to every doc version that includes them.
There are no per-release sample copies. Short field illustrations originally
authored inline remain inline.

`lines="5-20"` selects an excerpt from a file. Keep ranges and highlights aligned
when editing samples. Fern reads the included files during preview and publishing.

For a new release, copy the development pages into a new version directory and
adjust their relative include paths to the same shared source directory. Copy
`current.yml` to a version YAML file, update its page paths, resolve release-specific
values, and add a `path:` entry to `docs.yml`.

## Migration provenance

`migration-sources.json` records the source tags and conversion toolchain
revisions. Release `migration-audit.json` files retain source and output hashes,
YAML include selections, and Sphinx conversion evidence as static records of the
original import. Their historical sample paths and hashes predate the shared
source layout; they do not describe immutable copies in the current tree.

The 3.1.3 and 3.1.4 sources declared an Operator installation version of 3.1.2;
the imported examples and CRD URLs were corrected to their respective releases.
The 3.1.4 snapshot uses `nim-operator-v3.1.4-1`, which corrects the bug-fix link
to PR #977.

## CI

This checkout contains the docs CI callers under `.github/workflows/`:

- `fern-docs-preview-copy.yml` builds previews for copied PR branches.
- `fern-docs-preview-delete-copy.yml` cleans up previews when PRs close.
- `fern-docs-publish.yml` publishes after pushes to `main`.

They call NVIDIA/docs-builder's shared workflows and select the official Fern
release `5.216.0`. Preview and cleanup filters include development pages,
versioned pages, and `manifests/docs/**`.

Docs CI runs Node.js and the Fern CLI directly on GitHub-hosted runners.

`fern-docs-publish.yml` calls the shared publish workflow after pushes to `main`,
including merged pull requests. It publishes the version catalog, including Dev
from `current.yml`. Publish runs are serialized to avoid overlapping deployments.

Production publishing runs only in `NVIDIA/k8s-nim-operator` and uses
`DOCS_FERN_AWS_ROLE_TO_ASSUME` and
`DOCS_FERN_SECRET_ID`. The AWS role's OIDC trust must allow the
`fern-docs-publish.yml` caller on `refs/heads/main` and the shared
`NVIDIA/docs-builder/.github/workflows/_fern_docs_publish.yml@refs/heads/main`
workflow.
