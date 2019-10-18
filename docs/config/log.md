# Log functionality

Example configuration:

```
[log]
level = "debug"
mode = "json"
output = "/var/log/revad.log"
```

## Directives

```
Syntax:  level = string
Default: level = "debug"
```

`level` defines the log level, eg. "debug", "warn", "info"

```
Syntax:  output = string
Default: output = "stderr"
```

output sets the output for writting logs. The "stdout" and "stderr" strings have special meaning
as they will set the log output to stdout and stderr respectively. revad will create the filename 
specified in the directive if it does not exists. revad does not perform any log rotate logic, this task
is delegated to tools like *logrotate(8)* configured by the system administrator.

```
Syntax:  mode  = "dev" | "prod"
Default: mode  = "dev"
```

mode sets the format for the logs. dev mode sets the output to be consumed by humans on a terminal.
prod mode sets the output format to JSON so it can be parsed by machines and send to central logging systems
like Kibana.

