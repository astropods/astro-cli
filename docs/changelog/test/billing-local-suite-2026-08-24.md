# A local billing suite

## Summary

Billing had 215 tests and no way to run them as one thing. The suite was spread
across eight packages, and the parts that stop an account from spending were the
parts with no unit coverage at all.

`BillingSuspendWorker.Work` is the job that scales a gated account to zero. It
had none. `BillingResumeWorker.Work` had none. `StopNamespaceWorkloads`, the
call that does the scaling, was reachable only from the vcluster suite, so no
one ran it locally. The caps an account sets for itself were in the same state:
reads of a usage threshold were covered, but every write was not.

That combination is the worst one for this feature. The code that decides an
account should stop was well tested, and the code that stops it was not.

## Design

**One command.** `moon run astro-server:test-billing` runs the suite and prints
coverage over the billing sources. It needs no Postgres, Kubernetes, or browser.

Every package runs whole. Selecting billing tests by name is tempting, because
billing does not own the packages it lives in: the gating middleware sits with
every other middleware, the suspend job with every other River worker. But
billing test names in a shared package share no common token, so a name filter
drops `TestSelfLimitReached`, `TestCollectAfterCard`, and
`TestNoopProviderIsNotAProvisioner` without saying so. A slower run beats a
filter that skips.

Both runs measure the same `-coverpkg` set, so their profiles are merged by
summing each block rather than concatenated, which would list every block twice.

The task ends with a coverage number and the uncovered functions, so a gap reads
as output rather than as an absence.

**Two seams, both narrow.** Nothing in the suspension path was testable without
a live cluster, because the call took a concrete `*kubernetes.Clientset`.

`StopNamespaceWorkloads` now takes `kubernetes.Interface`. Its body already used
only interface methods, so every caller compiles unchanged, and the scale-to-zero
now runs against a fake clientset.

`BillingSuspendWorker` holds an optional override for that call. Nil selects the
real one, so production wiring is untouched, and a test can drive the loop past
the cluster to the status writes underneath. `BillingResumeWorker` takes its
queue through a one-method interface, the same narrowing `DunningSweepWorker`
already uses for the same reason.

**A dropped suspension now retries until it lands.** Writing the test exposed a
real gap. The suspend loop logged a deployment it could not stop and carried on,
then returned nil, so River recorded the job as done. Every site that enqueues a
suspension is edge-triggered, and the dunning sweep deliberately does not
re-suspend on each tick, so nothing came back for that deployment. It kept
running, and kept spending, for as long as the account existed.

The loop still attempts every deployment, so one unreachable cluster does not
strand the rest. It now collects the failures and snoozes rather than returning
them. A plain error would be discarded after MaxAttempts, which puts the
deployment back where it started; a snooze raises the ceiling with each retry,
so the job survives as long as the problem does. The retry re-reads only rows
still marked active, so it picks up exactly the deployments that did not stop.

**Suspension checks the account first.** A retry that never expires needs
something to end it. The job now reads the gating status before it touches a
cluster and returns without acting unless the account is still suspended. That
also narrows a race the old code had on its one retry path: recovery fires its
resume once, so a suspension landing after it would stop deployments that
nothing brings back.

Every transient failure takes the same route. A status read that fails is
neither recovery nor permission to act, and an unreadable deployment list means
the job has done nothing yet, so both snooze rather than returning an error the
25-attempt default would discard. One guarantee, one code path.

Two gaps in the surrounding machinery are worth naming, because this job cannot
close either from where it sits. The status check still races: a resume that
reads the deployment rows between the check and the write finds nothing to
restore, which needs the status read and the deployment writes to share a
transaction or a generation. And the enforcement switch that reacts to a status
change enqueues a suspend for `suspended` and a resume for `active`, but nothing
for `past_due`, so an account moving from suspended to dunning keeps its
deployments stopped until it reaches active.

**What the new tests hold.** Suspension reaches every active deployment, and an
unreadable deployment list fails the job rather than dropping the suspension. A
suspended deployment is marked `StatusSuspended` rather than `StatusStopped`, so
resume restores what billing stopped and leaves what the user stopped. The
gating reason reaches the timeline as JSON that a quote cannot break out of.

On thresholds: the two usage metrics stay separate, because they bill in
different units and folding them suspends on the wrong number. Rewriting an
unchanged cap writes nothing, because Metronome cannot update a threshold and a
recreate clears the alarm on an account that is already over. A changed cap
archives before it creates, so the replaced cap does not stay live.

Every new assertion was checked by mutation: the behaviour it names was broken
in the source, the test was confirmed red, and the source was restored.

## Migration

None. `astro-server:test` and `astro-server:test-integration` are unchanged.
