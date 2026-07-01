## Summary

Trace IDs in the observability table were visually abbreviated twice: once by the explicit middle-shortening formatter and again by CSS overflow. This made each row end with an extra ellipsis.

## Design

The Trace ID cell now treats the formatted trace ID as the final display string. It clips overflow without adding another ellipsis and positions the copy control independently so the hidden button does not consume inline width.

## Migration

No user action required.
