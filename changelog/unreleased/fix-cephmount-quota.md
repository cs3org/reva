Enhancement: Support getting quota from path ancestors

The previous code supported to obtained quota only from the path itself, but in some cases,
the path may not contain the quota information, but its ancestor may contain the quota information.

https://github.com/cs3org/reva/pull/5525
