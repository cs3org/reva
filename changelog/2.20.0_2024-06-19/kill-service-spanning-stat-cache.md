Change: Drop unused service spanning stat cache

We removed the stat cache shared between gateway and storage providers. It is constantly invalidated and needs a different approach.

https://github.com/cs3org/reva/pull/4542
