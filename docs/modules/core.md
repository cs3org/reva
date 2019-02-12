# Core functionality

Example configuration:

```
[core]
max_cpus = 4
log_file = "/var/log/revad.log"
log_mode = "prod"
```

## Directives

```
Syntax:  max_cpus = uint | "uint%"
Default: max_cpus = "100%"
```
If max_cpus is set it determines the available cpus to schedule revad processes.

```
Syntax:  log_file = string
Default: log_file = "stderr"
```

log_file sets the output for writting logs. The "stdout" and "stderr" strings have special meaning
as they will set the log output to stdout and stderr respectively. revad will create the filename 
specified in the directive if it does not exists. revad does not perform any log rotate logic, this task
is delegated to tools like *logrotate(8)* configured by the system administrator.

```
Syntax:  log_mode  = dev | prod
Default: log_mode  = dev
```

log_mode sets the format for the logs. dev mode sets the output to be consumed by humans on a terminal.
prod mode sets the output format to JSON so it can be parsed by machines and send to central logging systems
like Kibana.


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
