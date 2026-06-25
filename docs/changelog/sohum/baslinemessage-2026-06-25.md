## Summary

The review queue now gives reviewers a subtle coverage signal once the dataset reaches a usable eval grade. This helps users who are mid-review understand that their labeled examples have crossed a meaningful threshold without implying the queue is complete.

## Design

The signal appears in the top-right of the review queue card header as an outlined success badge with a check icon. It uses the existing status badge styling, success token, and body typography so it reads as a quiet state indicator rather than a new workflow step. The badge uses the shared tooltip primitive above the pill, with copy based on the current server-computed grade:

| Grade | Badge | Tooltip |
| --- | --- | --- |
| `A` | `Strong coverage` | `You've labeled a representative sample. Keep going to capture edge cases and strengthen future evals.` |
| `B` | `Good coverage` | `You've labeled a solid sample of traces. Keep going to capture edge cases and push toward an A.` |
| `C` | `Enough coverage` | `You've labeled enough traces to get started. Keep going to improve coverage and reliability.` |
| `D`, `F`, empty | No badge | No tooltip |

The readiness logic follows the grade state already computed for the dataset. `C` is the first visible coverage cue, `B` communicates good coverage, and `A` communicates strong coverage. Grades below `C` keep the header quiet and continue to rely on the grade panel for progress guidance. The client listens to the server-computed `grade` field in the existing dataset summary instead of inventing a separate minimum item threshold. This keeps the cue aligned with the scoring model that already blends volume, good/bad ratio, and failure coverage.

## Migration

No user action is required. Existing dataset summaries already include the grade needed to render the status.
