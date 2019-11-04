# Changelog

We are using [calens](https://github.com/restic/calens) to properly generate a
changelog before we are tagging a new release. 

## Create Changelog items
Create a file according to the [template](TEMPLATE) for each 
changelog in the [unreleased](./unreleased) folder. The following change types are possible: `Bugfix, Change, Enhancement, Security`.

## Generate the Changelog 
- execute `go run tools/prepare-release/main.go -version 10.0.0 -commit -tag` 
in the root folder of the project.