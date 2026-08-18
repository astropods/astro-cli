# Severity icons on the pod Events tab

## Summary

The pod Events tab conveyed each event's severity with only a thin colored bar,
which is easy to miss and unreadable for color-blind users. Each event now leads
with a labelled severity icon so severity reads from shape and a screen-reader
label, not color alone.

## Design

`EventSeverityIcon` maps an event to one of three icons: a red alert for events
that need action (`severity: "stuck"`), an amber triangle for other warnings
(`type: "Warning"`), and a muted info circle for normal events. Each icon carries
an `aria-label` ("Needs attention", "Warning", "Info") so assistive tech announces
the severity. Warning and stuck summaries are also bolded so the row that matters
stands out while scanning the list.

## Migration

None. Presentation-only change to the existing Events tab.
