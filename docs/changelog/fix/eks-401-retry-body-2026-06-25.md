# Fix: EKS write requests failing with "Content-Length … but only wrote 0 bytes"

## Summary

Deploys intermittently failed with `failed to ensure namespace: request declared
a Content-Length of N but only wrote 0 bytes` (and similar on other write calls
to the cluster). Reads were unaffected, so it surfaced as flaky deploys rather
than total loss of cluster connectivity.

## Design

Two compounding issues in the EKS token transport (`internal/k8s/eks.go`):

1. **Token validity vs cache mismatch (root cause).** The presigned STS token
   was signed with `X-Amz-Expires=60` (valid 60s) but cached and reused for 14
   minutes (`tokenExpiry`). After 60s the cached token is signature-expired, so
   every API call in the 60s–14m window got a 401 until the next refresh. Set
   the presign validity to 900s (the aws-iam-authenticator 15m cap) so the token
   stays valid for the full cache window and 401s become rare.

2. **401 retry dropped the request body (the actual error).** On a 401 the
   transport refreshes the token and retries, but `http.Request.Clone` copies the
   `Body` reader by reference — and the first attempt already consumed it. The
   retry therefore sent the `Content-Length` header with a 0-byte body. Reads
   (no body) replayed fine, masking the bug; writes (namespace Create/Update,
   ~393 bytes, every deploy) failed. The retry now rewinds the body via
   `req.GetBody` (which client-go sets on requests), and refuses to retry a body
   that can't be rewound rather than sending a torn request.

Both issues predate recent work; increased deploy activity simply exposed the
write path landing in the stale-token window.

## Migration

None. Server-side fix; no config or spec changes.
