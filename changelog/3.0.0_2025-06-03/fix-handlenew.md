Bugfix: handlenew failed to handle spaces id

HandleNew, which creates new office files etc., tried to parse the parent container ref via spaces, and had no fallback for non-spaces.
This is now fixed.

https://github.com/cs3org/reva/pull/5156