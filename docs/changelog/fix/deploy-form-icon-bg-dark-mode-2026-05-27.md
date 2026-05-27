# Deploy form icon backings flip in dark mode

## Summary

The icon backings in the deploy form's knowledge binding rows and the persistent-volume picker were hard-coded to `bg-slate-200`. The raw palette utility doesn't respond to the theme, so in dark mode the surrounding row flipped while the icon backing stayed light — a visible light square on a dark surface.

## Design

Swap the raw palette utility for the `muted` semantic token, which is the elevation step already used elsewhere for subtle icon chips on a panel surface and which flips automatically with the theme. The same swap is applied to the volume picker's hover state so it stays in the same token family.

## Migration

None.
