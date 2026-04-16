# Fix: ECR repository creation on first GitHub build

## Summary

GitHub-connected builds were failing with a 404 on the very first push because the ECR repository didn't exist yet. The CLI path works because it pushes through the registry proxy, which auto-creates repos on write operations. GitHub builds bypass the proxy entirely — BuildKit gets direct ECR credentials via IRSA and pushes straight to ECR.

## Design

Added `EnsureRepository` to the `githubbuild.Builder` that calls ECR `DescribeRepositories` / `CreateRepository` before submitting the K8s build job. The GitHub build worker (`GitHubBuildWorker.Work`) invokes this for each component destination, making it a retriable infrastructure error on failure.

The ECR client is behind an `ecrAPI` interface for testability, lazily created from AWS config when not injected.

Also added success/failure logging to the `ecr-login` init container shell command, which previously ran silently.

## Migration

No migration required. Existing agents that have already built successfully are unaffected — `DescribeRepositories` short-circuits when the repo already exists.
