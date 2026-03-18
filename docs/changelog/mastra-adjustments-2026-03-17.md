# Summary

This change unblocks local observability validation for Mastra-based agents by ensuring the local development stack provides a collector sink and newly scaffolded agents emit trace data using explicit Mastra observability wiring. The goal is to make the telemetry path predictable across local smoke tests and deployment workflows.

# Design

The local `ast dev` compose topology now includes an `astro-collector` service and injects OTLP routing into the agent runtime so emitted telemetry is sent to the local collector. Collector runtime metadata and Galileo credentials are forwarded from dev configuration into the collector container environment to support export behavior parity with deployment-side observability pipelines.

For Mastra scaffolding, newly generated agents now define an explicit observability entrypoint using Mastra `Observability` with an OTLP exporter and pass stable tracing metadata defaults in agent execution options. This moves new projects toward explicit observability configuration at agent construction time while preserving existing trace tagging behavior used by downstream processing.

To reduce local startup failures caused by host port collisions, the messaging gRPC host publish port was moved off the common `9090` binding and local agent wiring was updated accordingly.

# Migration

No server-side migration is required.

For existing agents, apply equivalent observability/exporter wiring in their agent code and ensure compatible Mastra package versions are installed before running local telemetry validation.
