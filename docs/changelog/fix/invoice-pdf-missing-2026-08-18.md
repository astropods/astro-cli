# Report a missing invoice PDF as missing

## Summary

Opening a finalized invoice could show "Couldn't load the invoice PDF" while the
server reported that the invoice was not finalized yet. Both statements were
wrong. The provider holds no PDF for some finalized invoices, so the absence is
the fact to report.

## Design

**The stage is not the reason.** `ErrInvoiceNotAvailable` was documented and
worded as "draft, so no PDF yet". A finalized invoice can also have no stored
PDF, in which case the old message contradicted the status badge next to it. The
server now answers "no PDF is available for this invoice" and the modal says the
same, so the two agree and neither claims to know why.

**The dialog names itself.** The invoice modal rendered a `DialogContent` with
no description, which Radix reports as a missing `aria-describedby`. A short
description carries the invoice period, so a screen reader announces which
invoice is open.

## Migration

None.
