Enhancement: validate OCM notification payloads

Add shared validation for canonical OCM notifications. Unknown
notification types remain accepted after the envelope checks, request
types answer 501 once valid, and shared secrets stay out of logs. The
sender may include a port, and errors name only known fields.
