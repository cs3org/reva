Enhancement: a framework for reminder kinds

The `reminders` service runs registered reminder kinds as jobs and keeps a SQL ledger of what it sent, so a recipient is told once per level of a condition. `reva reminders` shows the kinds, the open episodes and the history.

https://github.com/cs3org/reva/pull/5915
