Bugfix: use plain otel tracing in metadata client

In some cases there was no tracer provider in the context. Since the otel tracing has settled we will fix problems by moving to the recommended best practices. A good starting point is https://lightstep.com/blog/opentelemetry-go-all-you-need-to-know

https://github.com/cs3org/reva/pull/3950
