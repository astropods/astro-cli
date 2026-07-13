# Summary

Long deployment names could overwhelm deployment surfaces because the reveal, share badge, and dashboard card were designed around shorter labels than the previous display-name allowance. Deployment display names now use one 42-character limit shared by validation and badge rendering so names stay readable without breaking the existing card layouts.

# Design

Deployment display-name validation uses a 42-character limit at the API boundary after trimming whitespace. The deploy/configure form mirrors that limit with inline validation instead of a hard input `maxLength`: users can see what they typed, get an accessible error under the field, and cannot submit or save names until the name is within the accepted length. The same validation path applies to new deploys, renames, and redeploys. The client counts code points for this check, matching the server's rune-count behavior for emoji and other astral characters. The name field was also narrowed to match the intended label editing footprint rather than stretching across the whole form.

The live reveal uses the same 42-character display-name budget before rendering the headline, share label, and generated badge data. The reveal headline now also has bounded width, responsive sizing, and aggressive word wrapping as a defensive layout guard.

`astro-trading-card` now treats the badge title as a fixed 42-character visual budget rendered at a fixed title font size. The standard badge renders that budget into three centered title slots of 14 source characters each. Names are normalized before wrapping, breaks prefer whitespace, slash, hyphen, underscore, period, and camel-case boundaries when the remaining text still fits in the remaining slots, and unavoidable hard breaks append a hyphen as a continuation marker without dropping a source character. The SVG title block is clipped to the badge text lane as a last-resort guard, and the divider plus metadata rows move down under the full wrapped title block.

Badge stat rows now reserve a measured value lane to prevent label/value collisions. Regular stat values truncate inside that lane, account values keep their avatar-plus-handle layout, and call sites can explicitly mark origin-style values for right-aligned hyphen wrapping so long `account/name` origins remain visible across multiple lines without coupling renderer behavior to the display label text. Row heights and dividers expand with wrapped values instead of assuming every stat is one line, and the standard badge budgets the metadata region against the bottom barcode so wrapped stats and integration pills cannot overlap the barcode area.

Dashboard deployment cards intentionally stay close to the existing main-branch layout. Normal titles and slugs keep the same centered markup and classes as before; only titles longer than the dashboard card budget get a display-only ellipsis value before rendering. This avoids changing the visual rhythm of cards such as `Sohum's Slack Test Bot` and `Feature Flag Assistant` while preventing unusually long names from spilling out of the card. The configure page also short-circuits name-only saves when the inline display-name validation is active, avoiding a server round trip that would only return the same validation error.

The deployment detail header can now opt out of the deployment-menu fade mask, so the selected deployment name renders with ordinary ellipsis truncation next to the dropdown chevron on the detail page. Other hosts of the same menu, such as compact chat contexts, keep the existing faded behavior.

Coverage now exercises the validation path, deploy-form error handling, reveal/badge caps, SVG title wrapping, long `From` value wrapping, and dashboard-card behavior for both normal-length and over-budget titles.

# Migration

No action is required for existing deployments. Names that exceed the badge budget are capped defensively on reveal and badge surfaces as those views render. New or updated deployment display names must be 42 characters or fewer; callers that hit the validation error should shorten the display name and retry.
