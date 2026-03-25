Enhancement: add outgoing users management table

Added a new GORM-based SQL table for managing outgoing users (users leaving the organization).
The table tracks users in two states: grace period and archiving. Follows the same pattern as
the projects table.

https://github.com/cs3org/reva/pull/5555
