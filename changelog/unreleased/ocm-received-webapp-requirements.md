Bugfix: reject unusable received webapp shares

Incoming shares and open-in-app now require a compatible blank target,
a resolvable URI, and supported code-flow requirements. Offers that
require must-use-mfa are rejected until the current session is proven.
