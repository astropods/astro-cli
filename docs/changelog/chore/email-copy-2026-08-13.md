# Notification copy for `reason` and `details`

## Summary

Novu templates own the subject, body, and CTA of every notification, but two payload
properties are prose the server writes: `reason` and `details`. A template cannot phrase
them, because one observation workflow covers eight conditions and the wording depends on
which condition fired and on the observed value. Those two strings had drifted from product
copy toward diagnostics.

Three problems. Observation `details` mixed the observation and the fix into one long
sentence, and the same string is served to the alert catalog for conditions that are not
firing, where advice reads as a false alarm. Several titles used internal framing
(`Compute over budget`, `Memory over budget`) rather than what the reader can act on.
`build.failed` sent `cause.Error()` verbatim, so a reader could receive `go build: exit 1`
as the explanation of why their build failed.

## Design

**Observation details now read as claim, evidence, fix.** `Condition` gains a `Guidance`
field holding the fix as one imperative sentence. The evaluator composes
`Description` + `DetailsFor(value)` + `Guidance`, so each part has one job:

```
The agent is running close to its memory limit.        // Description: what is happening
Memory use peaked at 94% of the limit.                 // DetailsFor: the observed number
Raise the memory limit so the agent doesn't run out and stop.  // Guidance: the fix
```

Splitting `Guidance` out of `Description` is what makes the alert catalog correct.
`GET /deployments/:id/alerts` serves `Description` for every condition including green
ones, and it now describes what the rule detects without telling the reader to change a
limit that is fine.

`overProvisionedDetail` became a small factory taking the resource name, so the CPU and
memory conditions name their own resource instead of sharing an anonymous "what you
reserved". It also stopped suggesting a target size. The alert knows the usage ratio, not
the configured value, so "lower it to about 36% of its current value" asked the reader to
open the spec and multiply, at a precision a six-hour P95 does not support. The share of
the reservation that peak usage represents needs no arithmetic and already implies the
headroom; picking the new number belongs on the deployment page, which the CTA opens.

**Titles name the reader's problem.** `Restart storm` → `Frequent restarts`,
`Memory over budget` → `Near memory limit`, `Compute over budget` →
`Slowed by CPU limit`, and the two `over-provisioned` titles → `Unused CPU` /
`Unused memory`. `Crash loop` and `Out of memory` were already right. The workload
join dropped its em dash: `Frequent restarts (model-x)`.

`unschedulable` keeps the scheduling term as `Can't schedule`. A plainer title has to
name a cause, and the gauge behind the rule reports none: it fires for node capacity,
quota, taints, affinity, and topology constraints alike. The term is precise, the
audience is developers who know what a scheduler is, and it is the word to search for.
The description carries the symptom instead.

**`build.failed` no longer forwards Go errors.** `buildFailureReason` classifies the cause
with `errors.As` rather than by message prefix, so rewording an error cannot silently
change the bucket. A container build failure and an infrastructure failure each get a fixed
sentence, because the underlying text is compiler or network output that helps nobody in an
inbox.

A spec failure is the exception, and it is the one case where the raw error held something
the reader needed: the commit that lacked the file, or the line that failed to parse. A new
`githubbuild.SpecError` carries both phrasings, `Reason` for the reader and `Err` for the
log and the build record:

```go
SpecError{
    Reason: "astropods.yml has a syntax error on line 4: mapping values are not allowed in this context.",
    Err:    fmt.Errorf("parse spec YAML: %w", err),
}
```

`Reason` reaches the notification verbatim. Keeping the two apart means the build record and
the log are unchanged by copy edits, and a future permanent error that forgets to set
`Reason` falls back to a safe sentence rather than leaking.

Two details of phrasing carry logic rather than a fixed string. A YAML error's line number
moves into the prose, because the parser's own `line N:` behind our `astropods.yml is
invalid:` gave the reader two colons in one breath. A validation failure keeps the
validator's `field: message` form, which reads the way linter output does, and the lead
sentence ends in a period so that form is the only colon present. Validation problems are
also capped at three, with the remainder counted, since a spec can fail in a dozen places
and all twelve arrived on one line.

**`keyKind` matches the app.** It was `"OTel ingest"`, rendering as "New OTel ingest key
created", while the settings page calls the same credential an "Ingestion key". It is now
`"ingestion"`.

Auditing the rest of the notification surface turned up two payload variables that no call
site ever fills, so the templates render a blank on every send. Those are defects rather
than wording, and they ship separately on `fix/empty-notification-payloads`.

## Migration

None. No payload keys changed, so the Novu workflow payload schemas stay as they are.
Template authors should know that `details` is now up to three sentences rather than one,
and that neither `reason` nor `details` contains an em dash.
