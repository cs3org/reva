Enhancement: handle OCM notifications consistently

Validate incoming notifications and return an empty 201 on success.
Malformed notifications, including unknown types with an invalid
envelope, return 400; valid share requests return 501.
