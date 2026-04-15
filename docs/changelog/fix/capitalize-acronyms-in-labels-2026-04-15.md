## Summary

Variable field labels derived from environment variable names were rendered with incorrect casing for common acronyms — `SLACK_API_KEY` produced "Slack Api Key" and `WORKSPACE_IDS` produced "Workspace Ids".

## Design

`labelFromKey` already converts `SNAKE_CASE` keys to title case. A post-pass now replaces a fixed set of known acronyms (`Api → API`, `Id → ID`, `Ids → IDs`, `Url → URL`, `Oauth → OAuth`) as whole words, so the title-case result is corrected before display. The function is exported to allow unit testing.

## Migration

No action required.
