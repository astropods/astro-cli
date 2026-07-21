# Explain why Redeploy is disabled for paused agents

## Summary

A paused (stopped or scaled-down) agent cannot be redeployed, but the Configure page gave no signal: the Redeploy button did nothing and the user was left guessing. This adds a clear explanation and a one-click path forward.

## Design

The Configure footer now reads the live deployment status. When the agent is paused (status value `inactive`, with the deployment record as a fallback before the first status poll), the footer explains that the agent is paused and cannot be redeployed, and swaps the Redeploy/Save actions for a single Resume button. Resume reuses the existing wake-up mutation (the same one the top-bar status toggle uses), so the agent transitions back to active and the normal Redeploy footer returns. Keying off the live status rather than only the record means the footer leaves the paused state as soon as Resume is clicked, because wake-up optimistically marks the status deploying.

## Migration

None. The behavior change is limited to the Configure footer for paused agents.
