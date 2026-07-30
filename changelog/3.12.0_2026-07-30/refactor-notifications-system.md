Enhancement: Refactor notifications system

Following ADR general/0007-notifications-refactor

We now use an event-based system, where handlers register to events,
and a configurable rule-system determines how to handle these.
Accumulation is coordinated between daemons.

https://github.com/cs3org/reva/pull/5708