Enhancement: Port drone pipeline definition to starlark

Having the pipeline definition as a starlark script instead of plain yaml 
greatly improves the flexibility and allows for removing lots of duplicated
definitions.

https://github.com/cs3org/reva/pull/1528