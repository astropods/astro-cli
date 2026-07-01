## Summary

Renaming a conversation from the chat History list was unreliable: clicking Rename would open the inline edit field but then immediately close it, and typing could jump focus to another conversation — so the rename never took. Both symptoms came from the edit input living inside a Radix menu that fights for focus.

## Design

The row actions (Rename / Delete) previously lived in a nested Radix dropdown menu inside the History dropdown. Selecting Rename tore down that nested menu, and its focus-scope teardown (a deferred focus-restore) blurred the just-mounted edit input, firing the input's blur-to-commit and snapping the row back before the user could type. Separately, the menu's built-in typeahead intercepted single-character keystrokes bubbling from the input and moved focus to a menu item whose title matched — intermittently, only when the typed prefix matched another conversation.

The fix removes the nested overflow menu entirely. Rename and Delete are now plain inline buttons revealed on row hover, so triggering an action no longer opens or closes a menu — there is no focus scope to tear down and no focus race. The edit input mounts, focuses/selects via a stable callback ref, and stays put. Keystrokes also stop propagating to the surrounding menu so its typeahead can't hijack them. Delete continues to use the existing inline confirm row.

## Migration

No action required. The three-dot options menu on each conversation is replaced by inline rename/delete buttons that appear on hover; the rename and delete flows are otherwise unchanged.
