# Trim the balances comments

## Summary

Comment-only. The balances endpoint landed through the free-trial modal PR
carrying prose that restates the code it sits above: a handler doc naming the
route already in its signature, and three test comments explaining what their
own assertions say.

## Design

Each surviving comment keeps the one fact the code cannot state: that Metronome
returns a zero balance without `include_balance`, that omitting `CoveringDate`
widens the read to past and future grants, and that a credit type's label parses
as USD even when its id is wrong, so the id is what a test has to assert.

## Migration

None.
