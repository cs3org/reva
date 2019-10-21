---
title: "Overview"
linkTitle: "Overview"
weight: 1
description: >
  How can Reva help you?
---

## What is it?

Reva is an open source platform developed in the Go programming language with two purposes:

1. As an interoperability platform to link platforms with the storage and application providers.
2. As the reference implementation of the CS3APIS. 

#### The problem  we try to solve

Today any cloud sync and share provider either closed-source (Dropbox, Google Drive, ...) or open source (ownCloud, Seafile, ...) have their own catalogue
of integrations with many applicaition providers. The pitty about these integrations is that they are ad-hoc to the cloud vendor, making it impossible to port 
anywhere else.

{{< imgproc overview.png Fit "800x600">}}
{{< /imgproc >}}

#### The goal

Our ultimate goal is that any open source project and software vendor that provides either storage or applications to an EFSS platform
will adopt the CS3APIS as the facto standard way to collaborate. Having all the partners using the same APIs will enable all of us for a new dimension 
of portability of applications across vendors. 

{{< imgproc final.png Fit "800x600">}}
{{< /imgproc >}}

We know that for our vision to realize we need to allow time for the different stakeholders to adopt the CS3APIS.

In the mean time, we have another approach to make these systems talk to each other in a compatible way.

#### The approach with Reva

The motivation is to move out the ad-hoc APIs from the sync and share platform to Reva. As Reva is a vendor-neutral and open source
project we expect that many institutions and application providers will contribute with their ad-hoc APIs to Reva, creating an inter-operability platform 
that could potencially connect to an eco-system of applications and storage catalogs as never seen before.

The platform acts as middleware component in your current infrastructure, and by connecting your cloud to Reva you gain access
to multiple storage backends and multiple integrations with many application providers.

{{< imgproc reva.png Fit "800x600">}}
{{< /imgproc >}}

## Why do I want it?

* **What is it good for?**: Reva is good to avoid vendor locking in various ways: you could change your storage backend or the EFSS platform and your users
will still be able to use the same applications as before, making cloud migrations much transparent and friction-less. How many times have you been using an app
that when you move out to another cloud you have to loose access to it?

* **What is it not good for?**: Reva is not a replacement for a EFFSS or Cloud Platform, it is just a middleware component that enables inter-operability
and runs alongside your infrastructure.

* **What is it *not yet* good for?**: Reva is still very young to be used in production. We are working hard to reach a stable version soon with decent integrations to storage
and applications. We don't want to re-invent the wheel so will provide integrations with [rclone](https://rclone.org/) for storages and with other protocols out there like 
[WOPI](https://wopi.readthedocs.io/en/latest/) or [Learning Tools Interoperability](https://www.imsglobal.org/activity/learning-tools-interoperability) for compatible collaboration.

## Where should I go next?

* [Getting Started]({{< ref "docs/Getting Started" >}}): Get started with the project
* [Concepts]({{< ref "docs/Concepts" >}}): Deep-dive into the conceps behind Reva 

