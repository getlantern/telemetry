# telemetry
Common library for any Go code that wants to interface with telemetry.

This expects everything to be configured via environment variables to maximize portability and flexibility.

To modify the sample rate and sampling strategy, for example, you can use:

```
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.001
```

See https://opentelemetry-python.readthedocs.io/en/latest/sdk/trace.sampling.html