# Core functionality

Example configuration:

```
[core]
max_cpus = 4
log_file = "/var/log/revad.log"
log_mode = "prod"
```

## Directives

* ```max_cpus```: sets the maximun number of cpus used by revad. It is allowed to specify
the available cpus with percentages ("50%")
