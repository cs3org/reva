Enhancement: Decomposed FS: return a reference to the parent

We've implemented the changes from cs3org/cs3apis#167 in the DecomposedFS, so that a stat on a resource always includes a reference to the parent of the resource.

https://github.com/cs3org/reva/pull/2691
