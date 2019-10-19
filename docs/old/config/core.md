# Core functionality

Example configuration:

```
[core]
max_cpus = 2
tracing_enabled = true
```

## Directives

```
Syntax:  max_cpus = uint | "uint%"
Default: max_cpus = "100%"
```
If max_cpus is set it determines the available cpus to schedule revad processes.

```
Syntax:  tracing_enabled = boolean
Default: tracing_enabled = false
```

```
Syntax:  tracing_endpoint = string
Default: tracing_endpoint = "localhost:6831"
```

```
Syntax:  tracing_collector = string
Default: tracing_collector = "http://localhost:14268/api/traces"
```

```
Syntax:  tracing_service_name = string
Default: tracing_service_name = "revad"
```

```
Syntax:  disable_http = false | true
Default: disable_http = false
```

If disable_http is set to false, revad will not listen on the specified http network and address and 
http services will not be exposed by revad.

```
Syntax:  disable_grpc = false | true
Default: disable_grpc = false
```

If disable_grpc is set to false, revad will not listen on the specified grpc network and address and 
grpc services will not be exposed by revad.
