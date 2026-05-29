## Summary

When an AGENT.md frontmatter contained any invalid field — a malformed YAML token, a tag list over the 10-item limit, the wrong type for a single field — `ParseAgentCard` returned an error and the server silently dropped the entire agent card. The user got no feedback and the agent published with a blank card. Since AGENT.md is purely display metadata, refusing the whole document is the wrong default. We should accept whatever's valid and tell the user what was dropped.

## Design

`ParseAgentCard` is now best-effort and never returns an error. Its signature changed from `(*ParsedAgentCard, error)` to `*ParsedAgentCard` and the result always contains a body plus whatever fields parsed cleanly. Anything that didn't parse is recorded on a new `Warnings []string` field (tagged `json:"-"`, so it never leaks through the API).

Frontmatter is now decoded field-by-field through `yaml.Node` instead of one whole-document `yaml.Unmarshal`. A bad `description` only drops `description`. A bad item inside `tags` only drops that item. Overflowing the 10-tag limit truncates to 10 instead of erroring. Description and capability truncation also emit warnings now. If the frontmatter YAML is malformed at the syntax level, or the document isn't a mapping, the frontmatter is dropped wholesale but the body is still kept.

Because `ParsedAgentCard` contains only basic JSON-marshallable types, `json.Marshal` cannot fail, and every optional field is tagged `omitempty`. The minimum output is `{"body":""}` — dropped fields are simply absent from the JSON, never null or malformed. A new test asserts this property over malformed/garbage inputs.

The CLI surfaces warnings during `ast push` and `ast dev`: after the existing deprecated-meta-fields check, `warnDeprecatedMetaFields` now also parses the local AGENT.md and prints each warning to stderr with the same `⚠` formatting:

```
⚠  AGENT.md: tags: more than 10 provided, kept the first 10
⚠  AGENT.md: description: must be a string, dropped
```

## Migration

No action required. Existing AGENT.md files that previously parsed cleanly still parse identically. Files that previously caused a silent drop will now publish their valid fields and surface warnings in the CLI for the invalid ones.
