# SDK instrumentation telemetry forwarding

This module provides the SDK-facing listener used by the Datadog extension. It advertises `/telemetry/proxy/` at `/info` and accepts only `POST /telemetry/proxy/api/v2/apmtelemetry` for forwarding to instrumentation telemetry intake.

Forwarding preserves opaque native payloads, compressed bytes and supported SDK identity headers. The Collector supplies the API key and verified HTTP transport; caller authentication and connection-specific headers are discarded. Request size, aggregate in-flight bytes, concurrency and duration are bounded. Saturation returns 503 with a retry hint; intake status and Retry-After are returned without another queue or retries, with 404 and redirects mapped to 502.

Shutdown cancels in-flight forwarding and closes idle connections. Collector 0.164 / stable 1.70 is the tested minimum API. Component tests also exercise the current Collector version. This capability does not deliver application logs or dynamic instrumentation output.
