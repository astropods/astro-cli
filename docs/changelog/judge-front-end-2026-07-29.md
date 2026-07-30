# AI-assisted review queue

## Summary

Adds AI judgment predictions to the trace review workflow so reviewers can inspect, accept, or override suggestions without replacing human judgment.

## Design

The review queue forwards verdict filters to the cursor-paginated server API and renders prediction lifecycle and results alongside each trace. Queued and in-progress work is polled until it settles, with compact status indicators and an animated aggregate judging state.

Completed predictions expose the suggested verdict, confidence, rationale, and criterion assessments. Reviewers can agree with the judge, which preselects accepted or rejected predicted criteria, or choose a different verdict. Shared status badges, tooltips, and progress bars keep prediction details consistent with the wider interface.

Run AI Judge triggers server-selected asynchronous work without sending trace content from the client. The action refreshes queue state, links estimated usage to billing, and is unavailable while work is active or the fully loaded queue has no remaining predictions to request.

## Migration

No migration is required.
