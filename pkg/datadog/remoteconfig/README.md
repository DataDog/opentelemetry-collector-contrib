# Collector RC adapter

This module adapts the public Agent RC engine to the SDK-facing RC protocol and component lifecycle. The Datadog extension registers its handler on the shared `pkg/datadog/sdk` listener; the SDK listener does not depend on the RC engine.

The engine uses public snapshot `v0.84.0-devel.0.20261030015547-4fcf7d423463`, matching the Agent commit already used by contrib dependencies. No unmerged Agent changes, companion Agent or copied coordinator are required. The engine retains shared polling, signed configuration verification, caching, targeting and SDK response generation. Collector 0.164 / stable 1.70 is the tested minimum API; component tests also exercise the current version.

## Published-engine POC limits

The engine owns its backend HTTP client. Basic proxy settings pass through the extension's Agent configuration bridge and normal HTTPS verification remains enabled. Collector custom CA/mTLS settings and HTTP instrumentation do not apply to that RC client. `request_timeout` bounds SDK-facing requests, not background backend calls, which lack adapter-imposed total deadlines, shutdown cancellation and explicit idle-pool cleanup. The client follows Go's default redirect policy; a stalled backend can delay updates and shutdown.

Use the fixed Datadog backend and system-trusted TLS for this POC. Standalone Collector identity and backend health reporting, production transport/lifecycle behavior and broader compatibility still need agreement. The compatibility Agent-version build flag remains POC scaffolding. SDK instrumentation telemetry forwarding and DI delivery retain their independent HTTP controls; OpAMP and direct OTLP DI intake are future work.
