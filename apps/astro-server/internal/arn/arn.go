// Package arn provides helpers for constructing and parsing Astro ARNs.
//
// ARN format: arn:{type}:{account-short-id}:{name}
//
// The account short ID is a stable 16-char hex value derived from the account
// UUID via FNV-64a. It is shorter than the full UUID while retaining enough
// entropy to make collisions negligible.
package arn

import (
	"fmt"
	"hash/fnv"
)

// AccountShortID returns a stable 16-char hex ID derived from the full account
// UUID using FNV-64a. Used in ARNs and K8s resource names.
func AccountShortID(accountID string) string {
	h := fnv.New64a()
	h.Write([]byte(accountID))
	return fmt.Sprintf("%016x", h.Sum64())
}

// KnowledgeStore returns the ARN for a managed knowledge store.
func KnowledgeStore(accountID, name string) string {
	return fmt.Sprintf("arn:knowledge:%s:%s", AccountShortID(accountID), name)
}
