---
title: HTTP API
description: Every HTTP endpoint of a reva deployment, in Swagger UI.
weight: 10
layout: swagger
notoc: true
---

Reva describes its HTTP API from the routes its services declare, so the document cannot drift from the code. Every deployment that enables the [`openapi` service](/docs/components/http-services/openapi/) serves its own document at `/openapi.json` and this viewer at `/openapi`.

This page shows the document of the CERNBox deployment. Your deployment serves only the services it enables, so its document can differ.
