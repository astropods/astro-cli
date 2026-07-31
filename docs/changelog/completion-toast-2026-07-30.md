# AI judge completion feedback

## Summary

Notifies reviewers when an AI judging run finishes, including when some items could not be judged.

## Design

The client follows deployment-wide prediction lifecycle counts after a judging run starts. Once no predictions remain queued or in progress, it compares the submitted count with newly completed predictions. Successful runs confirm that scored traces are ready to review, partial failures show retry guidance, and fully failed runs show an error notification. Individual failed predictions remain visible with a “Couldn’t judge” badge in the queue and a warning banner in the selected trace detail. The review queue uses independent list and detail columns so detail feedback never changes the position of list controls.

## Migration

No migration is required.
