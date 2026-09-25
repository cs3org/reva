Security: enforce transport TLS floor and scheme policy

The shared OCM HTTP transport now requires TLS 1.2 or newer on every
client (including trusted and insecure modes), rejects plain HTTP on
public-only clients except an explicit literal-loopback opt-in, and
rejects HTTPS-to-HTTP redirects on public-only HTTP clients. The
address classifier also closes bounded gaps for the IANA special-purpose
ranges (0.0.0.0/8, 198.18.0.0/15, 192.0.2.0/24, 198.51.100.0/24,
203.0.113.0/24, 240.0.0.0/4, 64:ff9b:1::/48, 2001:db8::/32), keeping
the PCP and TURN anycast exceptions (RFC 7723/8155) and the well-known
NAT64 unwrap.

Explicit federation CIDR ranges do not relax that scheme policy, the
TLS minimum, or redirect enforcement. A configured private address is
still HTTPS-only, and a redirect is checked against the same address
policy as the dial.

https://github.com/cs3org/reva/pull/5839
