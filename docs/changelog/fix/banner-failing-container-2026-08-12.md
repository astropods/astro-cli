# Name the failing container in the deploy failure banner

## Summary

When an agent has more than one container (for example an agent plus a collector
sidecar) and one of them fails to come up, the deploy failure banner named only
the generic cause. Users couldn't tell which container broke, and a healthy
container alongside the failed one made the failure message look wrong.

## Design

The failure banner folds the workload that failed into its header instead of
adding a separate line, so it stays a single clear heading. It reads the failing
`component` (falling back to `workload`) from the `failed_on` entry the status
endpoint already returns, drops the "Action required" prefix (the yellow warning
banner already implies urgency), and appends the component, for example "Image
pull failed for the agent container". That points at the specific container to
fix while keeping the banner to a heading plus one line of guidance. The
actionable remediation copy and the pull-vs-generic classification were handled
earlier in #1719; this closes the remaining part of the issue.

## Migration

None. Presentation-only, driven by data the banner already receives.
