Enhancement: stop claiming background jobs on a drained node

Draining a node pauses its jobs runner, and running jobs that implement `HandOffJob` are asked to move to another node.

https://github.com/cs3org/reva/pull/5874
