# HTTP Service: prometheus

This service exposes a [Prometheus](https://prometheus.io/)
telemetry endpoint so metrics can be consumed.

To enable the service:

```
[http]
enabled_services = ["prometheus"]
```

Example configuration:

```
[http.services.prometheus]
prefix = "metrics"
```

## Directives

```
Syntax:  prefix = string
Default: prefix = "metrics"
```

prefix specifies where the service should be exposed.
For example, if the prefix is "myservice", it will be
reachable at http://localhost:9998/myservice
