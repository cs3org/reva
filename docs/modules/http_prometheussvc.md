# HTTP Service: prometheussvc

This service exposes a [Prometheus](https://prometheus.io/)
telemetry endpoint so metrics can be consumed.

To enable the service:

```
[http]
enabled_services = ["prometheussvc"]
```

Example configuration:

```
[http.services.prometheussvc]
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
