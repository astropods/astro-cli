## Summary

Reviewing eval queue traces now supports keyboard verdicts so reviewers can grade traces without moving between the trace content and footer buttons.

## Design

The review queue maps `G`, `B`, and `N` to the existing Good, Bad, and Neutral judgment flow. Shortcuts use the same mutation and grade-flight animation path as button clicks, ignore repeated keydown events, and do not fire while focus is inside editable fields.

The footer exposes the shortcuts as key hints next to the verdict actions so the interaction is discoverable without changing the review queue layout.

## Migration

No migration required.
