---
title: Community
description: Reva is built in the open by CERN and contributors from many organisations. You are welcome to join.
layout: community
---

<div class="cards four">
  <a class="card" href="https://github.com/cs3org/reva/issues"><h3>Report a bug or ask for a feature</h3><p>Open an issue on GitHub. Search the existing issues first.</p></a>
  <a class="card" href="https://gitter.im/cs3org/reva"><h3>Chat</h3><p>Ask questions and meet the developers on Gitter.</p></a>
  <a class="card" href="https://github.com/cs3org/reva/labels/junior-job"><h3>Find a first task</h3><p>Issues labelled <em>junior-job</em> are a good start.</p></a>
  <a class="card" href="https://www.cs3community.org"><h3>CS3 community</h3><p>The community of research and education clouds behind the CS3 APIs.</p></a>
</div>

## Contribute code

1. Open an issue to discuss the change, unless it is small.
2. Fork the repository and create a branch.
3. Build and test with `make revad`, `make test-go`, and `make lint`.
4. Add a changelog entry in `changelog/unreleased/`.
5. Open a pull request.

The [contribution guide](https://github.com/cs3org/reva/blob/master/CONTRIBUTING.md) has the details. Please follow the [code of conduct](https://github.com/cs3org/.github/tree/master/CODE_OF_CONDUCT.md).

## Improve these docs

Most of the [component reference](/docs/components/) comes from the code. To document a package, add a doc comment or a `README.md` to it and run `make gen-doc`. The other pages are Markdown files in [`docs/content`](https://github.com/cs3org/reva/tree/master/docs/content).
