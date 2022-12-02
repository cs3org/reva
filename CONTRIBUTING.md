Contributing
================

First of all, thank you for your contribution!

There are several areas where you can help, the [OpenSource Guide](https://opensource.guide/how-to-contribute/)
is a nice guide emphasizing that not all contributions need to be code. 

We also have a [Code of Conduct](https://github.com/cs3org/.github/tree/master/CODE_OF_CONDUCT.md)
that is worth reading!

Please **[open an issue first](https://github.com/cs3org/reva/issues/new)** for any bug report or new feature if there isn't
already one opened. We use GitHub issues to keep track of failures in the
software and addition of new features. A GitHub issue is a nice place to discuss ideas
and get feedback from other members of the project.

If you have general questions or you want guidance on how to start contributing
you can reach us on [Gitter](https://gitter.im/cs3org/REVA).

If you want to find an area that currently needs improving have a look at the
open issues listed at the [issues page](https://github.com/cs3org/reva/issues). 

For newcomers, issues with minor complexity are tagged 
as [junior jobs](https://github.com/cs3org/reva/labels/junior-job).


Reporting Bugs
==============

If you've found a bug thanks for letting us know!
It is a good idea to describe in detail how to reproduce 
the bug (when you know how), what environment the bug appeared and so on.
Please tell us at least the following things:

 * What's the version of binary you used? Please include the output of
   `revad --version` or `reva version` in your bug report.
 * What commands did you execute to get to where the bug occurred?
 * What did you expect?
 * What happened instead?
 * Do you know how to reproduce the bug?

As more information you provide, the earlier we'll correct it!.

Development Environment
=======================

The repository contains several sets of directories with code: `cmd/` and
`pkg/` contain the main source code files.

The dependencies of REVA are kept in the `go.mod` file in the root of the repository.
In general we don't like to add new dependencies to the project and we try to stick as much
as possible to the standard Go library. In cases this is not convenient, we accept only 
external libraries that have a permissive license.


To compile the REVA daemon and the REVA command line you can:

```
$ git clone https://github.com/cs3org/reva
$ make
$ ./cmd/revad/revad --version
version=v0.0.0 commit=639f48d branch=review go_version=go1.19 build_date=2019-04-17T13:57:17+0200 build_platform=linux/amd64
```

If you only want to run the tests you can:

```
$ make test
```

Take a look at the Makefile in the root of the repository for all the available options.

Providing Patches
=================

The workflow we're using is described on the
[GitHub Flow](https://guides.github.com/introduction/flow/) website, it boils
down to the following steps:

 0. If you want to work on something, please add a comment to the issue on
    GitHub. For a new feature, please add an issue before starting to work on
    it, so that duplicate work is prevented.

 1. First we would kindly ask you to fork our project on GitHub if you haven't
    done so already.

 2. Clone your repository locally and create a new branch.
    If you are working on the code itself, please set up the development environment
    as described in the previous section.

 3. Then commit your changes as fine grained as possible, as smaller patches,
    that handle one and only one issue are easier to discuss and merge.

 4. Push the new branch with your changes to your fork of the repository.

 5. Create a pull request by visiting the GitHub website, it will guide you
    through the process.

 6. You will receive comments on your code and the feature or bug that they
    address. Maybe you need to rework some minor things, in this case push new
    commits to the branch you created for the pull request (or amend the
    existing commit, use common sense to decide which is better), they will be
    automatically added to the pull request.

 7. If your pull request changes anything that users should be aware of (a
    bugfix, a new feature, ...) please add an entry to the file
    [CHANGELOG.md](CHANGELOG.md). It will be used in the announcement of the
    next stable release. While writing, ask yourself: If I were the user, what
    would I need to be aware of with this change.

 8. Once your code looks good and passes all the tests, we'll merge it. Thanks
    a lot for your contribution!

Please provide the patches for each bug or feature in a separate branch and
open up a pull request for each.

We enforce the Go style guide. You can run `make lint` to fix formatting
and discover issues related with code style.

Every time you create a pull request, we'll run differents tests on Travis.
We won't merge any code that doesn't pass the tests. If you need help to make the test
pass don't hesitate to call for help! Having a PR with failing tests is nothing
to be ashamed of, in the other hand, that happens regularly for all of us.

Git Commits
-----------

It would be good if you could follow the same general style regarding Git
commits as the rest of the project, this makes reviewing code, browsing the
history and triaging bugs much easier.

Git commit messages have a very terse summary in the first line of the commit
message, followed by an empty line, followed by a more verbose description or a
List of changed things. For examples, please refer to the excellent [How to
Write a Git Commit Message](https://chris.beams.io/posts/git-commit/).

If you change/add multiple different things that aren't related at all, try to
make several smaller commits. This is much easier to review. Using `git add -p`
allows staging and committing only some changes.


For example, if your work is done on the reva daemon, prefix the commit
message with `revad: my message`, if you work on the cli prefix it with `reva: my message`,
if you work only on the package directory, prefix it with `pkg/pkgname: my message` and so on.

Code Review
===========

We encourage actively reviewing the code so it's common practice 
to receive comments on provided patches.

If you are reviewing other contributor's code please consider the following
when reviewing:

* Be nice. Please make the review comment as constructive as possible so all
  participants will learn something from your review.

As a contributor you might be asked to rewrite portions of your code to make it
fit better into the upstream sources.
