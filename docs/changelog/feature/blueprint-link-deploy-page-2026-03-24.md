# Deploy Page Kebab Menu Improvements

## Summary

Adds contextual actions to the deploy page kebab menu and aligns it with the agent cards menu on the Your Agents page.

## Design

- **View blueprint** link added as the first menu item, navigating to the agent's blueprint page (`/:account/:agentSlug`).
- **Share agent badge** added, opening the existing `TradingCardModal` with the deployed agent's card data — matching the same action already available on agent cards.
- **Copy build number** replaces "Copy ID number".
- **Icons** swapped from lucide to Heroicons outline across both the deploy page kebab and agent card menus.
- **Kebab trigger** updated to `EllipsisHorizontalIcon` and `text-foreground` for better visibility.
- **Menu moved** to sit directly next to the agent name in the deploy page header.
- **Agent card menu** updated to match: same items, same spacing (`10px 14px` padding), same icons.
- **Dropdown background** unified to `--popover` (stone-25) across both menus.
- **Agent badge modal** now closes on background click.

## Migration

No migration required.
