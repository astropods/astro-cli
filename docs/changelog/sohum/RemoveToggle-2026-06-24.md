## Summary

The eval dataset experience no longer offers a Pretty/Raw display toggle. Reviewers now see formatted content by default in both the review queue and dataset tabs, which keeps the interface simpler and avoids exposing a mode that did not add much value for the workflow.

## Design

The review queue and dataset item details now send content through the existing pretty render path directly. The toggle state and props were removed from the tab headers, table row plumbing, stories, and tests so there is a single display contract for eval trace previews.

## Migration

No migration is required. Existing dataset and review queue data continues to render through the pretty formatter.
