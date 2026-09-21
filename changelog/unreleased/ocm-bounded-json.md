Security: bound peer-controlled OCM JSON

Every OCM control-plane response body and peer-controlled error snippet is
now capped at a 64 KiB default limit through `readOCMBody` and
`decodeOCMJSON` helpers, which read the body once and return
`ErrResponseTooLarge` for oversized input. This prevents decode-after-drain
regressions in `NewShare` and `InviteAccepted` and bounds the existing
unsigned directory JSON at the same limit.

https://github.com/cs3org/reva/pull/5834
