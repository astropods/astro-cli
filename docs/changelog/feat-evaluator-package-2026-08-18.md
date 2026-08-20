# Evaluator execution package

## Summary

The eval dataset replaces its single fixed judge with a set of independent evaluators,
each answering one defined question and returning one typed value. `internal/evaluator`
adds the execution surface for one such evaluator, so the preset registry, trace-level
runner, result tables, and review-queue APIs build against a settled contract.

## Design

An evaluator definition and one trace go in, and one typed result comes out. The package
validates the definition, assembles a payload from the trace and whatever context the
definition asks for, calls the model through the AI gateway, and validates what comes
back against the declared output type.

A definition declares what it needs rather than how to get it. Its context configuration
selects from previous turns, the next user message, user feedback, and the trace's steps,
and only what it selects reaches the model. Its output declares a boolean, enum, number,
or string, which drives both the response schema and the check applied to the answer.

Definitions are data, not code. The prompt travels as payload rather than as part of the
system instruction, so an evaluator can describe a new question without changing how
evaluators run, and adding a context option does not mean rewriting the instruction.

## Migration

None. The package has no callers yet: no endpoint, no rows, no jobs. `evaljudge` and the
existing prediction path are untouched.
