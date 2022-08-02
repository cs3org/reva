Enhancement: pass an extra query parameter to WOPI /openinapp with a
unique and consistent over time user identifier. The Reva token used so far
is not consistent (it's per session) and also too long.

https://github.com/cs3org/reva/pull/2155
https://github.com/cs3org/wopiserver/pull/48
