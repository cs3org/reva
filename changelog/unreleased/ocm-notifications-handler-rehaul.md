Enhancement: validate and log received OCM notifications

`POST /notifications` now decodes notifications into the canonical contract
instead of logging the raw body. A non-JSON body, malformed JSON, a missing
notificationType, a body over 1 MiB, or a known type with invalid fields after
legacy normalization gets 400. REQUEST_SHARE and REQUEST_RESHARE get 501.
Valid known types, legacy-shaped payloads and unknown types, including MLS
types while MLS is not supported, get an empty 201. Tolerance of legacy
shapes is unconditional and adds no configuration key. Received
notifications are only logged with structured, redacted fields: the
notification.sharedSecret and the raw payload are never logged, and nothing
is changed or sent in response.
