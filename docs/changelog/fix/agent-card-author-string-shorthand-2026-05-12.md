## Summary

When an AGENT.md file used a plain-string array for `authors` (e.g. `authors: ["Alice"]` or the YAML block equivalent), the entire frontmatter parse failed. `BuildAgentCardJSON` swallows parse errors by returning an empty string, so the frontend received no card data and silently showed a blank AGENT.md UI.

## Design

`AgentCardRepo` already handles both string and object forms via a custom `UnmarshalYAML`. The same pattern is now applied to `AgentCardAuthor`: a scalar YAML node is accepted as the `Name` field; a mapping node decodes as before. This means all of the following are equivalent:

```yaml
# object form (existing)
authors:
  - name: Alice
    account: alice

# string shorthand (now supported)
authors:
  - Alice

# inline JSON-style (now supported)
authors: ["Alice"]

# mixed
authors:
  - Alice
  - name: Bob
    account: bob
```

## Migration

No action required. Existing AGENT.md files using the object form are unaffected. Files that previously caused a silent parse failure will now render correctly.
