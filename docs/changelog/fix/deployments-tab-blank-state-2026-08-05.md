# Fix blank Deployments tab during a fresh deploy

## Summary

Clicking Redeploy from the Configure form navigates to the Deployments tab, but
the new deployment has no pods yet, so the main content area rendered a bare "No
active pods" message that read as a blank, broken page — even though the header
correctly showed "Deploying".

## Design

`AgentDeployments` renders the pod graph from the merged spec + runtime workload
list. When that list is empty it now distinguishes two cases: if a deploy is in
flight (status `deploying`, or the status query hasn't resolved yet) it shows a
spinner with "Deploying your agent…"; otherwise it keeps the existing "No active
pods" idle message. This guarantees the page never looks blank while pods are
still coming up.

## Migration

None. Presentation-only guard on an existing view.
