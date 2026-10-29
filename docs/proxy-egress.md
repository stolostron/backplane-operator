# Provider proxy egress

The CAPOA and CAPK controller NetworkPolicies use the `HTTP_PROXY` and
`HTTPS_PROXY` environment variables configured for the MultiClusterEngine
resource to add egress access to the proxy.

When a proxy URL contains an IP address, the rendered NetworkPolicy permits
egress to that IP address on the configured proxy TCP port. When that port is
443 or 6443, the policy also permits egress to any destination on that port
because the template has unconditional TCP rules for those ports. IPv4
addresses produce a `/32` CIDR, and IPv6 addresses produce a `/128` CIDR.

When a proxy URL contains a hostname, Kubernetes NetworkPolicy peers cannot
select that DNS name. The renderer therefore permits egress to `0.0.0.0/0` and
`::/0` on the configured proxy TCP port. This means the selected controller
pods can connect to any destination on that port, not only to the configured
proxy host.

Use a proxy IP address in the MultiClusterEngine proxy configuration when a
stricter NetworkPolicy is required. Hostname addresses are not resolved at
render time because proxy DNS records can change independently of the rendered
NetworkPolicy.
