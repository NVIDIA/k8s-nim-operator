# Documentation inputs and example output

Dev, 3.1.3, and 3.1.4 all include these files using Fern's native `Code` component.
Changes to these shared sources apply to every version that references them.

- `input/` contains 42 standalone YAML examples, copied from the current Dev sources.
  Documentation-specific examples are at its root; NIM examples retain `nim/`
  subdirectories.
- `output/` contains all nine files from the original Sphinx `docs/manifests/output/`
  directory at tag `nim-operator-v3.1.4-1`. Four files provide eight excerpts in
  each docset; the other five are retained as source examples.

Test the inputs directly in the relevant Kubernetes or Helm workflow. Some files
contain several alternatives separated by `---`, Helm values, or placeholders
that must be configured before use. The output files are captured examples, not
commands to apply. Keep excerpt ranges and highlights aligned when editing files.

From `fern/pages/`, include a sample with paths such as:

```mdx
<Code src="../../manifests/docs/current/input/nim/serving/standalone/basic/retriever-embedding.yaml" language="yaml" />
<Code src="../../manifests/docs/current/output/get-nimcache.txt" language="output" />
```

From `fern/versions/v3.1.4/pages/`, the equivalent paths start with
`../../../../manifests/docs/current/`.

