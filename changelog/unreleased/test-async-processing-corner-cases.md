Enhancement: Test async processing cornercases

We added tests to cover several bugs where file metadata or parent treesize might get corrupted when postprocessing errors occur in specific order.
For now, the added test cases test the current behavior but contain comments and FIXMEs for the expected behavior.

https://github.com/cs3org/reva/pull/4625
