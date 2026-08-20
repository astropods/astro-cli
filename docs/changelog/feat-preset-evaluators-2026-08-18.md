# Preset evaluator registry

## Summary

`internal/evaluator` can execute an evaluator definition, but nothing produces
definitions yet. Astro needs a set it owns: something every dataset can run on day one,
before anyone writes an evaluator of their own, and something publication can validate
references against.

`internal/evalpreset` adds that registry. Six evaluators and a default set that groups
them, addressed by stable references and held in code, so a dataset gets useful
evaluation without configuration and every deployment runs the same definitions.

## Design

The presets target defects an agent builder can act on, not general answer quality. A
builder can already read a trace and judge whether the answer was good. What they cannot
see at a glance, and cannot check by hand across thousands of traces, is whether the
agent leaked personal data or a credential, revealed its own configuration, wasted a tool
call, or stated something its own steps do not support.

`preset/default-evaluation` covers those:

| Key | Detects |
|---|---|
| `exposed_pii` | Personal data about someone other than the requester |
| `leaked_credentials` | A usable key, token, or password rather than a placeholder |
| `disclosed_system_instructions` | The agent's prompt, rules, tools, or model |
| `unnecessary_tool_call` | A tool ran whose result did not shape the answer |
| `claim_grounding` | Whether specific claims trace back to the agent's own steps |
| `user_sentiment` | How the user reacted to the response |

Each one answers a single question and returns a typed value, so a result points at one
fix rather than a score a builder has to interpret.

References are the unit of addressing. A dataset names a set, a set names its evaluators,
and both resolve through the registry. Holding definitions in code rather than the
database means every environment runs identical presets and a change ships as a code
review, and it leaves the door open for database-backed definitions to resolve through
the same reference later.

## Migration

None. Presets have no callers yet, and no agent resolves a set until the evaluator worker
lands.
