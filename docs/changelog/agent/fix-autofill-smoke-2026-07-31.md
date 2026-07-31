# Summary

The production secrets smoke test expected a plaintext saved variable to auto-fill a secret deployment field. Type-compatible deploy autofill correctly prevents that unsafe binding, leaving the test out of date with the product behavior.

# Design

The smoke fixture continues to create both a secret and a plaintext account variable. The deploy check now verifies that the compatible secret reference auto-fills while the plaintext reference does not fill the secret Weather field. This preserves settings coverage for both variable types and exercises the deploy-time compatibility boundary.

# Migration

No migration is required.
