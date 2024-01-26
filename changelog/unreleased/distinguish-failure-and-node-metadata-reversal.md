Bugfix: distinguish failure and node metadata reversal

When the final blob move fails we must not remove the node metadata to be able to restart the postprocessing process.

https://github.com/cs3org/reva/pull/4481
