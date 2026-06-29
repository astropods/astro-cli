# Summary

Temporarily skip the production smoke assertion for the signed-out `/explore` navbar while production catches up to the client route fix.

# Design

The smoke test remains in place but is marked skipped, preserving the expected signed-out navbar contract in code without failing the production suite against an older deployed client build.

# Migration

No user action required.
