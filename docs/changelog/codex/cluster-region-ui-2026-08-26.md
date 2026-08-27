# Compact region selection

## Summary

The deployment form presented each allowed region as a separate card. Flags, checkmarks, and repeated borders made a single-choice field visually heavy.

## Design

The form now presents multiple regions as compact native radio rows inside one bordered group. The group follows the form width on deploy and configure pages. Each row shows the customer-facing region name, infrastructure region code, and default status. Parenthetical locations use muted text to keep the directional region name prominent. The default marker uses a neutral badge treatment. The selected row uses a neutral background and radio dot. Region flags are omitted because they do not add placement information.

The helper text appears before the choices. Existing deployments also show migration guidance before the region list when the selected region changes. Accounts with one allowed region see a selected, disabled value. A tooltip explains how to request another region.

Field headers now share one spacing pattern across forms. Titles sit close to their helper text, while controls have more separation from the helper text.

## Migration

No action required.
