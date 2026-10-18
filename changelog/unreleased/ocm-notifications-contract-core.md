Enhancement: add a portable OCM notification contract

The ocmd package gains a reusable notification core with no reva adapter
imports. It decodes received notifications into one canonical form and
classifies them as spec-shaped, legacy-shaped, unknown, malformed, MLS or
request types. Legacy payloads without senderDomain or with the deprecated
top-level providerId are normalized, with notification.file.providerId taking
precedence. The known types are validated per type, the optional
notification.sharedSecret is kept for comparison and redacted from every
string, JSON and log form, and RESHARE_UNDO is validated like SHARE_UNSHARED
while keeping its own identity. A handler registry, a nil-able MLS ingress
seam and a pure fire-once sender state machine are included. Nothing is wired
to HTTP yet.
