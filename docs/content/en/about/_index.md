---
title: About
# description: Bridging clients, application providers and cloud storages
layout: about
aliases: ["/history/", "/community/"]
---

## What is Reva?

Reva is an interoperability platform consisting of several daemons written in Go. It acts as bridge between high-level clients (mobile, web, desktop) and the underlying storage (CephFS, EOS, local filesytems).  It exports well-known APIs, like WebDAV, to faciliate access from these devices. It also exports a high-performance gRPC API, codenamed the [CS3 APIs](https://buf.build/cs3org-buf/cs3apis), to easily integrate with other systems. Reva is open source. It was started at CERN and is still actively developed at CERN, but we also welcome outside contributors to join us.

<div class="cards four">
  <a class="card" href="https://www.cs3community.org"><h3>CS3 community</h3><p>The community of research and education clouds behind the CS3 APIs.</p></a>
  <a class="card" href="https://github.com/cs3org/reva/issues"><h3>Report a bug or ask for a feature</h3><p>Open an issue on GitHub. Search the existing issues first.</p></a>
  <a class="card" href="https://gitter.im/cs3org/reva"><h3>Chat</h3><p>Ask questions and meet the developers on Gitter.</p></a>
  <a class="card" href="https://reva-community.web.cern.ch"><h3>Forum</h3><p>Read announcements and discuss your setup on the community forum.</p></a>

</div>

## History

{{< timeline >}}

Since that day, different organisations and individuals have joined the project to shape the future of collaboration.

**The next chapter is written by you.**

## Improve these docs

Most of the [component reference](/docs/components/) comes from the code. To document a package, add a doc comment or a `README.md` to it and run `make gen-doc`. The other pages are Markdown files in [`docs/content`](https://github.com/cs3org/reva/tree/master/docs/content).
