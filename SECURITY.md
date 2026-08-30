# Security policy

## Supported versions

NexaGate is currently an early `0.2.x` project. Only the latest commit on the
`main` branch is supported while the design is being stabilized.

## Reporting a vulnerability

Please use GitHub's private **Report a vulnerability** / Security Advisory
feature when it is available for this repository. Do not publish a working
exploit, panel password, certificate private key, REALITY private key, WARP
profile, user database, or connection URI in a public issue.

Include the affected commit, Linux distribution, architecture, relevant
service logs with credentials removed, expected behavior, and a minimal way to
reproduce the problem.

## Security boundaries

The design assumes the server's root account and hosting control panel remain
trusted. NexaGate isolates its runtime services and blocks unintended direct
egress from Xray, Hysteria, and DNS, but it cannot protect traffic after a root
compromise or guarantee the behavior and availability of Psiphon, Cloudflare
WARP, Let's Encrypt, or other third-party infrastructure.
