Enhancement: handle OCM notifications consistently

Validate incoming notifications and return an empty 201 on success.
Malformed known types return 400; unsupported share requests return 501.
