Enhancement: receive OCM notifications

Validate and authenticate incoming notifications. Return an empty 201 for
handled or ignored notifications, 400 for malformed input, and 501 for
valid unsupported requests. Report handler and dependency failures as 500
without exposing handler errors or peer response bodies.
