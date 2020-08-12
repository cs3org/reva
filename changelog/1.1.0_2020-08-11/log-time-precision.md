Enhancement: Improve timestamp precision while logging

Previously, the timestamp associated with a log just had the hour and minute,
which made debugging quite difficult. This PR increases the precision of the
associated timestamp.

https://github.com/cs3org/reva/pull/1059
