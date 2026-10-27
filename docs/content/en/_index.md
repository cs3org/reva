---
title: Reva
description: Reva is an open-source server that connects storage, applications, and other clouds through the CS3 APIs.
headline: Connect storage, applications, and clouds
subline: Reva is an open-source server written in Go. It serves your existing storage over WebDAV, OCS, and the CS3 APIs, opens files in web apps, and shares them with other clouds. It powers CERNBox at CERN.
features:
  - title: Plugins for everything
    text: Storage, users, groups, shares, and authentication are drivers. Pick one in the configuration, or write your own in Go.
    url: /docs/components/drivers/
  - title: Built on the CS3 APIs
    text: All services talk gRPC through the CS3 APIs. Components in other languages can join the same system.
    url: /docs/api/grpc/
  - title: Federated sharing
    text: Open Cloud Mesh (OCM) lets users share with people on other sites, such as other universities.
    url: /docs/components/grpc-services/ocmshareprovider/
  - title: Proven at scale
    text: CERNBox serves tens of thousands of users and petabytes of data with reva on EOS.
    url: /deployments/
  - title: Clients you already have
    text: Desktop and mobile sync clients and web front ends work through WebDAV, OCS, and the Graph API.
    url: /docs/components/http-services/
  - title: Open source since 2019
    text: Apache 2.0 licensed and developed in the open on GitHub. New contributors are welcome.
    url: /community/
---
