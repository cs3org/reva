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
