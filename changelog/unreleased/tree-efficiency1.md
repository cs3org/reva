Enhancement: Use more efficient ReadDir method in loop.

Use a more efficient implementation from the library with ReadDir and save
an additional stat call with that in a for loop over all dir content.

https://github.com/cs3org/reva/pull/3114
