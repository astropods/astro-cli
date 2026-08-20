# Classifier calls carry 64 prompts, not 256

## Summary

A day of prompt classification failed with `context deadline exceeded` on every
tick. `Classify` posted up to 256 prompts in one request against a 60 second
timeout, and the work took longer than the timeout allowed. The client cancelled,
the server finished the batch anyway, and the day retried on the next tick and
failed the same way. `maxBatch` drops to 64.

## Design

The cost of a call is the cost of its tokens. Claude Code prompts nearly all
reach the classifier's truncation length, so a batch of 256 is 256 full-length
sequences through the encoder, with no short prompts to average the cost down. A
predictor scores about six of those a second, which puts a 256-prompt call at
roughly 82 seconds against a 60 second deadline. Measured in production: the
server logged `:predict` at 82.5 seconds and returned `200` to a caller that had
given up 22 seconds earlier.

Batch size does not buy throughput here. Batching pays only while per-call
overhead dominates, which stops at about 8 texts for this model. Past that the
graph is compute-bound and the cost per text is flat, so 256 in one call and 64
in four calls cost the same total work. The only thing the larger batch changes
is how much of that work sits behind a single deadline.

64 puts a call near 11 seconds, which leaves room for a slower or busier
predictor without reaching the timeout. Four calls where there was one adds
about 40 milliseconds of HTTP overhead to a day, against the risk of losing the
day.

The infrastructure side of the same failure is fixed separately: the predictor
now sorts by length before it chunks, sizes its onnxruntime thread pool to its
CPU quota, and runs on 8 cores. Those changes took the same 256-prompt call from
148 seconds to 43. This change stops one deadline from covering the whole day's
work.

## Migration

None. No configuration changes and no API changes. Days that failed earlier
retry on the next tick, because the producer keeps a failed day out of the
watermark.
