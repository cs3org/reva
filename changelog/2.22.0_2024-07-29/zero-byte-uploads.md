Bugfix: Zero byte uploads

Zero byte uploads would trigger postprocessing which lead to breaking pipelines.

https://github.com/cs3org/reva/pull/4778
