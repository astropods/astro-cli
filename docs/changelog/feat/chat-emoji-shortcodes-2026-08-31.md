# Summary

Two changes to how a conversation reads in the chat.

Emoji shortcodes rendered as their source text, so a message arrived as
":tada: shipped it" rather than "🎉 shipped it". Slack sends shortcodes
literally, an agent that copies a Slack thread into a user's history copies them
verbatim, and agents emit them in their own replies. The chat now renders a
shortcode as the emoji it names.

The API behind those copied threads was undocumented. `StreamOptions`
carries `saveConversation` and `getThreadHistory`, which together let an agent
bring a conversation in from anywhere and put it in a user's chat history, and
neither appeared anywhere in the public docs. The capability is generic and
already available to any agent on the messaging sidecar, so the docs were the
only thing missing.

# Design

## Emoji

The chat renders markdown through Streamdown, which already takes a plugin list,
so this is `remark-gemoji` on that list rather than a pass over the text. Gemoji
resolves the same shortcode table GitHub uses, which is also where Slack's
standard names come from.

Passing `remarkPlugins` to Streamdown replaces its own list rather than adding to
it, and the defaults carry GFM. They go back in ahead of gemoji, since dropping
them would take tables and strikethrough with them. Streamdown exposes those
defaults as a keyed record, not a list, so the spread reads its values.

Gemoji rewrites only text nodes, and only names in its table. A shortcode inside
a code span or a fenced block is left as typed, and so is a clock time like
10:30:45. A Slack custom emoji has no unicode form and no entry in the table, so
`:partyparrot:` still arrives as text; naming those would need the workspace's
own emoji list, which the chat does not have.

The tests assert both halves: emoji resolve, and the GFM defaults they were
appended to still work.

## Saved conversations doc

A new Messaging SDK page documents saving a conversation from another source, and
the `StreamOptions` tables on both custom-adapter pages gain the two fields.

The page is organized around the parts an agent author gets wrong rather than
around the type signature. The identity rule comes first, because a save
addressed to a raw platform id is refused. Then the idempotency key, because the
conversation id is derived from it and that is what makes a save repeatable
rather than duplicating a copy per source message. Then the conflict policy and
the status, because the status is the reason the call is awaited: a copy can be
deleted or diverged, and only the agent can decide what to do next.

The page states version floors for the two adapter packages that expose the
fields, since the sidecar side has been serving the call for longer than the
published SDKs have exposed it.

`sourceLabel` and `sourceUrl` are documented as recorded with the copy rather
than displayed. The store keeps both, but the conversation list response carries
only `conversation_id`, `title`, and `updated_at`, so nothing renders them today.
A copied conversation is identified in the list by its title alone.

# Migration

None.
