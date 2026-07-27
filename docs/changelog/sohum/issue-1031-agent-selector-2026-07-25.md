# Search all agents from the quick selector

Ticket: [#1031](https://github.com/astropods/astro/issues/1031)

## Summary

The agent selector now keeps search at the top so people can find an agent directly without leaving their current page, even when their agents span several accounts.

## Design

The selector filters the existing cross-account deployment summary in the client, matching agent display names, agent identifiers, account display names, and account identifiers. Matching an account keeps all of its agents visible, while an agent match removes empty account groups.

Search stays fixed above the results, while the uncapped catalog scrolls inside the shared Radix scroll-area primitive. The non-modal selector uses searchbox and listbox semantics, gives every action a keyboard path, and focuses the field when it opens. The current account leads the catalog and the current agent leads that account; remaining accounts retain the shared personal-first and alphabetical ordering. Chat eligibility, navigation routes, and the single-agent deploy action remain unchanged. No ranking query, per-account cap, new API, or server work is required.

## Migration

None.
