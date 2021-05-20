Bugfix: Use the -static ldflag only for the 'build-ci' target

It is not intended to statically link the generated binaries
for local development workflows. This resulted on segmentation
faults and compiller warnings.

https://github.com/cs3org/reva/pull/1718
