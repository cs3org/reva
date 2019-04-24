# HTTP Service: helloworldsvc

This service is for demo purposes.
It exposes a single endpoint that renders a
hello message that can be changed in the configuration file.

To enable the service:

```
[http]
enabled_services = ["helloworldsvc"]
```

Example configuration:

```
[http.services.helloworldsvc]
hello_message = "Ola Mundo!"
```

## Directives

```
Syntax:  prefix = string
Default: prefix = "helloworldsvc"
```

prefix specifies where the service should be exposed.
For example, if the prefix is "myservice", it will be
reachable at http://localhost:9998/myservice


```
Syntax:  hello_message = string
Default: hello_message = "Hello World!"
```

hello_message defines the message that will be rendered under the
http endpoint configured by the **prefix** directive.

