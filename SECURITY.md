# Security Policy

## Supported versions

Only the latest release receives security fixes. Please upgrade before reporting.

## Reporting a vulnerability

Report vulnerabilities privately using GitHub private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**. Please do not open a public issue or discussion.

Include the version, how it is deployed, and steps to reproduce. This is a volunteer project. Expect an acknowledgement within about a week and a fix or mitigation plan as time allows. Please allow reasonable time before public disclosure.

## Scope and deployment notes

rigctlWeb is designed for use on a trusted LAN. It can key a transmitter, so access to it is access to your station.

- Never expose rigctlWeb or rigctld directly to the internet.
- Set an auth token and keep it private. Treat it like a password.
- For remote access, put rigctlWeb behind a TLS reverse proxy, or reach it over Tailscale or WireGuard.
- Bind rigctld to localhost unless another machine on your LAN truly needs it.
- rigctld has no authentication of its own. Anyone who can reach its port can control the radio.

In scope: authentication bypass, remote code execution, cross-site scripting or request forgery in the web UI, and anything that allows unintended transmit. Out of scope: issues that require direct, unauthenticated internet exposure contrary to the guidance above, and bugs in Hamlib itself (report those to Hamlib).
