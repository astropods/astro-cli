# Releasing @astropods/theme

The `@astropods/theme` package is published to npm automatically when a change to `packages/astro-theme/**` is merged into `main`. Publishing uses OIDC trusted publishing — no npm token is stored in GitHub secrets.

## How the release works

1. A merge to `main` that touches `packages/astro-theme/**` triggers the **Publish Theme** workflow.
2. The workflow reads the `version` field from `packages/astro-theme/package.json`.
3. It checks whether that version already exists on npm (`npm view @astropods/theme@<version>`).
4. If not yet published, it builds the package and runs `npm publish --provenance`.
5. If already published, it exits cleanly with a notice — no error, no re-publish.

## Releasing a new version

> **Warning:** The version is NOT bumped automatically. If you merge changes to `packages/astro-theme` without bumping the version, the workflow will detect that the current version is already published and skip the release silently. You must manually increment the version before merging.

1. On a new branch, bump the version in `packages/astro-theme/package.json`:
   ```json
   "version": "0.1.2"
   ```
2. Merge the branch to `main`.
3. The **Publish Theme** workflow fires automatically and publishes the new version.

To trigger a publish manually (e.g. if the automatic run was skipped), go to **Actions → Publish Theme → Run workflow** and run it from `main`.

## Prerequisites (one-time setup)

npm trusted publishing must be configured for the package before OIDC auth will work:

1. Log in to npmjs.com and open the `@astropods/theme` package settings.
2. Go to **Access → Trusted Publishers** and add a new publisher:
   - **Owner:** `astropods`
   - **Repository:** `astro`
   - **Workflow:** `publish-theme.yml`
3. Leave **Environment** blank.

Once configured, the workflow authenticates via the GitHub OIDC token — no stored secrets needed.
