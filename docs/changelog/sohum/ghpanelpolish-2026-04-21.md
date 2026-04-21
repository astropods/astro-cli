# Polish: GitHub connection panel UI

## Summary

A series of UI improvements to the GitHub connection panel on the blueprint detail page, driven by iterating through all states in Storybook.

## Design

**Connected header:** When a repo is linked, the section header now shows a persistent green checkmark + GitHub login (`✓ acme`) pulled from `repo_full_name`. The repo name in the connected view drops the owner prefix — just `my-repo` instead of `acme/my-repo`.

**Build rows:**
- Title is the commit message; sha and duration are removed from the card surface (will live in the logs dialog)
- Active builds: row 2 shows `⟳ Fetching spec (1/3)` — actiony step label + step count
- Succeeded builds: row 2 shows `✓ bld_abc123 successful` in green with a timestamp tooltip on hover
- Failed builds: row 2 shows `✕ Error: see logs for more` in red with a timestamp tooltip; full error string removed
- "Build Logs" moved from a standalone icon button into the parent `•••` dropdown as the first item

**Awaiting spec state:** Copy updated to "Awaiting `astropods.yml`".

**RepoPicker labels:** Repository and Branch fields now have labelled headers with `Github` and `GitBranch` icons — consistent across both the blueprint wizard and the connect-repo dialog.

**SidebarSection:** New `trailing` prop for right-aligned header content.

**Stories:** Added `GitHubConnectionPanel.stories.tsx` covering all 11 sidebar states (not connected, connecting, awaiting spec, pending, building at each step, succeeded, failed, history, disconnecting).

## Migration

No action required.
