# Push Git provenance

This document is the maintainer contract for Git metadata collected by
`ast push`. User-facing behavior is summarized by `ast docs help`; this document
defines the path scope, push gate, and registration payload precisely.

## Scope and timing

Git state is collected immediately before the build step of a normal push. The
relevant paths are:

- the directory containing the selected spec, including when `--file` points
  outside the current directory; and
- every declared component `build.context`, resolved from that spec directory.

Each directory is a recursive Git pathspec. Staged, unstaged, untracked
nonignored, and normal submodule changes count as dirty. Ignored files do not.
The status command is:

```text
git --no-optional-locks status --porcelain=v1 --untracked-files=normal -- <paths...>
```

Paths are grouped by repository and every discovered repository is checked.
The recorded commit always comes from the repository containing the selected
spec. Because one commit cannot describe multiple repositories, otherwise
successful checks across multiple repositories still represent incomplete
provenance.

## Push decisions

| Git state | Push behavior | Registration metadata |
|---|---|---|
| Every relevant path was checked and is clean | Continue | Send independently available commit fields and `working_tree_dirty: false` |
| Any relevant path is dirty | Stop before build, push, or registration unless `--allow-dirty` is present | When allowed, send independently available commit fields and `working_tree_dirty: true` |
| No dirtiness was found, but a path or status command could not be checked | Continue; warn when a commit SHA is available because that commit may not represent every input | Send independently available commit fields and omit `working_tree_dirty` |
| The commit lookup fails, including an unavailable or unborn `HEAD` | Apply the status result normally; the commit failure alone is nonfatal | Omit unavailable commit fields; serialize dirty state according to the status result |
| Git is missing or a relevant path is not in a repository | Continue unless another successfully checked relevant path is dirty | Send independently collected metadata; send `true` for dirtiness found elsewhere, otherwise omit the unknown dirty state |
| `--no-build` is used | Do not inspect Git or apply the dirty-input gate | Omit all Git provenance fields |

Detected dirtiness takes precedence over incomplete provenance. If one relevant
path is dirty while another cannot be checked, the push is still blocked unless
`--allow-dirty` is present, and an allowed push sends
`working_tree_dirty: true`.

The dirty gate is deliberately noninteractive. `--allow-dirty` is its only
bypass; `--yes` affects unrelated confirmation prompts and never bypasses the
gate.

## Registration contract

The registration request may contain:

| Field | Serialization rule |
|---|---|
| `commit_sha` | Send only when nonempty. It is the full `HEAD` object ID from the selected spec's repository. |
| `commit_message` | Send only when nonempty. It is the full `HEAD` message, including its body, with trailing line endings removed. |
| `working_tree_dirty` | Send `true` for detected dirtiness, `false` only for a fully verified clean path set, and omit when status is unknown or provenance is incomplete. |

Commit and status lookups are independent. A failed commit lookup does not
discard a known dirty or clean result, and a failed status lookup does not
discard an available SHA or message. Partial metadata is therefore expected.

Commit messages are truncated to at most 8 KiB without splitting a UTF-8 code
point. The server accepts nonempty SHA values only when they are 40- or
64-character hexadecimal object IDs and applies the same 8 KiB message limit.

Git provenance is registration metadata only. `ast build` does not collect it,
and commit messages are not written to OCI image labels.

## Rationale

- **Block known-dirty normal pushes:** the images are about to be built from
  the checked inputs, so silently associating them with `HEAD` would claim a
  reproducibility guarantee the commit cannot provide. `--allow-dirty` makes
  the exception explicit and preserves that fact for the Builds UI.
- **Allow incomplete checks:** missing Git, repositories with no `HEAD`, paths
  outside Git, and command failures should not make `ast push` unavailable.
  Omitting the dirty state lets the server and UI distinguish unknown
  provenance from verified-clean provenance.
- **Omit provenance for `--no-build`:** the current checkout cannot prove how
  an already-existing image was produced. Attaching its current `HEAD` would
  risk recording unrelated source metadata.
