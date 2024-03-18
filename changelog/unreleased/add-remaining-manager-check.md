Enhancement: Move more consistency checks to the usershare API

The gateway now checks if there will be at least one space manager remaining before
deleting a space member. The legacy ocs based sharing implementaion already does this
on its own. But for the future graph based sharing implementation it is better to have
the check in a more central place.

https://github.com/cs3org/reva/pull/4585
