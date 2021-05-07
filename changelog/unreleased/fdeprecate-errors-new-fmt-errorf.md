Enhancement: Deprecate using errors.New and fmt.Errorf

Previously we were using errors.New and fmt.Errorf to create errors.
Now we use the errors defined in the errtypes package.

https://github.com/cs3org/reva/issues/1673