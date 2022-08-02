Enhancement: Enhance storage registry with virtual views and regular expressions

Add the functionality to the storage registry service to handle user requests
for references which can span across multiple storage providers, particularly
useful for cases where directories are sharded across providers or virtual views
are expected.

https://github.com/cs3org/cs3apis/pull/116
https://github.com/cs3org/reva/pull/1570
