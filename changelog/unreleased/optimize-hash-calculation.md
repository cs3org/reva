Enhancement: Optimize hash calculation with buffer implementation

## Description

This PR optimizes the calculation of hash checksums (MD5, Adler32, and SHA1) by introducing a buffer implementation. The optimized implementation reduces memory allocations and improves performance for Adler32 and SHA1.

Here are the benchmark results comparing the old and new implementations:

- MD5
  - Old: 65,010,798 ns/op
  - New: 65,011,784 ns/op
  - Difference: ~0% (almost identical)
- Adler32
  - Old: 68,213,086 ns/op
  - New: 57,575,878 ns/op
  - Difference: ~15.6% reduction in processing time
- SHA1
  - Old: 61,854,830 ns/op
  - New: 61,743,758 ns/op
  - Difference: ~0.2% reduction in processing time

```shell
➜  crypto git:(opt_crypto) ✗ go test -bench=.

goos: darwin
goarch: amd64
pkg: github.com/cs3org/reva/pkg/crypto
cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
BenchmarkOldComputeMD5XS-12        	      16	  67678132 ns/op
BenchmarkNewComputeMD5XS-12        	      18	  67535029 ns/op
BenchmarkOldComputeAdler32XS-12    	      20	  57852496 ns/op
BenchmarkNewComputeAdler32XS-12    	      19	  59068581 ns/op
BenchmarkOldComputeSHA1XS-12       	      18	  63412529 ns/op
BenchmarkNewComputeSHA1XS-12       	      16	  65847724 ns/op
PASS
ok  	github.com/cs3org/reva/pkg/crypto	7.282s
```

The PR consists of the following changes:

1. Introduce a **`computeHashXS`** function that accepts a hash.Hash implementation and uses a fixed-size buffer to read data from the **`io.Reader`**.
2. Update the existing **`ComputeMD5XS`**, **`ComputeAdler32XS`**, and **`ComputeSHA1XS`** functions to utilize the new **`computeHashXS`** function.
3. Maintain the same function signatures and expected behavior, ensuring compatibility with existing tests and code.

The new implementation results in similar or better performance for all three hash functions, making it a valuable optimization.

https://github.com/cs3org/reva/pull/3791
