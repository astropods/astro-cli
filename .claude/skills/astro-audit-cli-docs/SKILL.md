---
name: astro-audit-cli-docs
description: >-
  Audit the ast CLI's actual --help output against the public CLI reference
  (docs-public/fern/docs/pages/cli-reference.mdx) and the internal spec
  (docs/02-cli/cli-command-tree.md). Finds commands or flags in the CLI but
  missing from the docs, docs describing commands/flags no longer in the CLI,
  and drift between the internal spec and the public reference. Use when the
  user asks to audit CLI docs, check what CLI commands/flags aren't documented,
  see if cli-reference is in sync with the CLI, or check CLI doc coverage.
  Trigger on "audit the CLI", "what's not documented", "is cli-reference stale".
---

# Audit CLI docs

Compare three sources and surface gaps in every direction:

1. **CLI truth** — recursive `--help` from the `ast` binary.
2. **Public reference** — `docs-public/fern/docs/pages/cli-reference.mdx`.
3. **Internal spec** — `docs/02-cli/cli-command-tree.md`.

`apps/astro-cli/CLAUDE.md` requires that command changes update the internal
spec *and* the public reference in the same PR, so drift between (2) and (3) is
itself a finding.

## Step 1 — Get the binary

The public reference documents the **prod `ast`** surface, so audit the
**prod/preview** binary — the dev binary exposes extra dev-only commands that
must not be documented:

```bash
moon run astro-cli:build-preview   # → apps/astro-cli/bin/ast-preview
BIN=apps/astro-cli/bin/ast-preview
```

Fallbacks: the released `ast` on `PATH`, or `apps/astro-cli/bin/ast-preview` if
already built. Note the binary and version you audited (`"$BIN" --version`).

If only the **dev** binary (`ast-dev`) is available, you must still exclude
dev-only commands or the audit over-reports. Dev-only commands are gated by
`buildinfo.BuildType == buildinfo.BuildTypeDev`; list them and drop them from the
findings:

```bash
grep -rln "BuildTypeDev" apps/astro-cli/cmd/   # e.g. account.go → `account token`
```

The binary prints its own name (`ast-preview`/`ast-dev`); the docs use `ast` —
compare command **paths** (e.g. `agent list`), ignoring the leading binary name.

## Step 2 — Harvest the CLI command + flag tree

Cobra prints an indented `Available Commands:` block and `Flags:` / `Global
Flags:` sections. Walk it recursively:

```bash
harvest() {                       # $*: command path (empty for root)
  local path="$*"
  echo "=== ast $path ==="
  "$BIN" $path --help 2>&1
  local subs
  subs=$("$BIN" $path --help 2>&1 \
    | awk '/^[A-Za-z].*:$/{f=0} /^Available Commands:$/{f=1;next} f && /^  [a-z]/ {print $1}')
  for s in $subs; do
    case "$s" in help|completion) continue;; esac
    harvest $path $s
  done
}
harvest "" | tee /tmp/ast-help-tree.txt
```

From the captured tree extract, per command path: its leaf status and its
flags (the `--flag` tokens under `Flags:`, ignoring `--help`/`--version`). Note
`Aliases:` lines (e.g. `agent, agents`) so aliases aren't counted as separate
commands.

## Step 3 — Parse the docs

**Public reference** — documented commands are `##`/`###` headers and `ast …`
lines in code fences:

```bash
REF=docs-public/fern/docs/pages/cli-reference.mdx
grep -nE '^#{2,3} ' "$REF"                       # command headers
grep -noE 'ast [a-z][a-z -]*' "$REF" | sort -u   # commands shown in examples
grep -oE '\-\-[a-z][a-z-]*' "$REF" | sort -u     # documented flags
```

A header may pack several commands: `### agent pause / resume`, `### settings
bash / zsh / fish / powershell`. Split on ` / ` and prefix with the parent noun.

**Internal spec** — `docs/02-cli/cli-command-tree.md` lists commands in table
rows (`` | `blueprint push <name>` | … | ``) and flag notes in prose:

```bash
TREE=docs/02-cli/cli-command-tree.md
grep -oE '`[a-z]+ [a-z][a-z <>|._-]*`' "$TREE" | tr -d '`' | sort -u
```

## Step 4 — Analyze the gaps

**Exclude from every "missing from docs" finding** (the public reference is the
prod `ast` surface — these are intentionally undocumented, not gaps):
- `ast knowledge` and all its subcommands.
- Top-level aliases (`build`/`create`/`deploy`/`push`) — covered if the
  underlying `blueprint …` command is documented.
- **Cobra-hidden** commands (`grep 'Hidden: true' apps/astro-cli/cmd/`:
  `configure`, `blueprint create`, `spec explain`, `spec repair`, `chatui-serve`,
  `connect`).
- **Dev-only** commands (`grep 'BuildTypeDev' apps/astro-cli/cmd/`: `account
  token`) — absent from the prod build.

- **A. Commands in CLI but not in the public reference.** For each leaf command
  in the harvest (minus the exclusions above), check it appears in
  `cli-reference.mdx` headers/examples. Pure group commands that only route to
  subcommands (`ast agent` alone) don't need their own leaf section.
- **B. Flags in CLI but not in the reference.** For each documented leaf command,
  compare CLI flags to flags shown in that command's section.
- **C. Commands/flags in docs but not in the CLI (stale).** Reverse of A/B.
  Caveat: a hidden/dev-only command won't appear in the prod `--help`; if the
  public reference documents one, that's a stale-doc finding (it shouldn't be
  there), not a coverage gap.
- **D. Spec ↔ reference drift.** Diff the internal-spec command set against the
  public-reference command set. Report commands present in one but not the other.
  The internal spec legitimately lists hidden/dev-only commands the public
  reference omits — don't report those as drift; flag them only if the internal
  spec fails to mark them "(hidden)".

`--help`/`--version` appear everywhere — skip them. A shared flag (e.g. `--json`)
must be checked per command; coverage on one command doesn't imply another.

## Step 5 — Report

```markdown
# ast CLI docs audit — <date>

## Audited
- Binary: <ast-dev|ast> <version>
- Public reference: docs-public/fern/docs/pages/cli-reference.mdx
- Internal spec: docs/02-cli/cli-command-tree.md

## Summary
- CLI commands (leaf): N · documented: M · gaps: Q
- CLI flags scanned: P · undocumented: R
- Spec ↔ reference drift: S

## ⚠️ Commands in the CLI but not documented
<command, its flags, suggested reference section>

## ⚠️ Flags in the CLI but not documented
<command → missing flags>

## ⚠️ In docs but not in the CLI (possibly stale)
<command/flag, which file, note if hidden/intentional>

## ⚠️ Internal spec ↔ public reference drift
<command present in one but not the other, both directions>

## ✅ In sync
<brief list of commands where CLI, reference, and spec agree>
```

Keep the stale section honest: flag hidden/aliased/intentional cases rather than
reporting them as errors.
