---
title: Security
description: Keep rigctlWeb on your LAN, sign-in and tokens, remote access, and what it never sends anywhere.
order: 5
---

# Security

rigctlWeb can key your transmitter, so access to rigctlWeb is access to your station. It's built
for a trusted home network, and it starts locked down.

## Defaults

- It listens on `127.0.0.1:8480`, your own computer only, until you start it with
  `-listen 0.0.0.0:8480` (or another address).
- Every page and API call needs the **access token**, which is generated on the first run and kept
  in the config file. Treat it like a password.
- Signing in sets a session cookie that is HTTP-only and SameSite=Strict, and is derived from the
  token rather than holding it. Sign-in attempts are rate-limited per address.
- The page has a strict Content Security Policy and talks only to its own server. Live updates use
  a same-origin WebSocket.

## Using it from other devices

To reach it from a phone or another computer on your LAN, listen on all addresses and keep the
token private. For anything beyond your LAN:

- **Never expose rigctlWeb or rigctld directly to the internet.**
- Use a VPN such as [Tailscale](https://tailscale.com) or WireGuard, or
- put it behind a reverse proxy with TLS (Caddy, nginx) and, ideally, the proxy's own sign-in.
  rigctlWeb marks its cookie Secure when the proxy says the connection is HTTPS
  (`X-Forwarded-Proto: https`). Use `-base-path` to serve it under a path.

## rigctld has no password

Anyone who can reach `rigctld`'s port can control the radio, bypassing rigctlWeb entirely. Start
it with `-T 127.0.0.1` unless another machine really needs it, and firewall it if so.

## Read-only radios

Mark a radio **Read-only** in **Settings → Radios & connection** to monitor it without any risk of
changes: rigctlWeb then refuses every command, preset and raw CAT command for that radio.

## What rigctlWeb never sends anywhere

- No telemetry, analytics, crash reports or update checks.
- Nothing about your station, radio or settings leaves your computer.
- The only outside request is **Update library** on the Presets page, made when you press it: a
  plain download of the public preset library from openRMS Rigs, identifying itself only as
  `rigctlweb/<version>`.
- Receive audio goes only to the browser that's listening, and only while it listens.

## Transmit protections

See [Transmit safety](panel.md#transmit-safety) and, for presets and raw CAT,
[Radio presets](presets.md#raw-cat-menu-settings). In short: a transmit timeout, it unkeys only
what it keyed, the public API never keys, and presets never key.

## Inside hamstack

When [hamstack](https://github.com/ATCUSA/hamstack) runs rigctlWeb, rigctlWeb listens on
localhost only, turns its own sign-in off and accepts only requests carrying hamstack's proxy
secret, and uses the radios hamstack gives it (read-only unless you allowed transmitting in
hamstack). It refuses to start if anything it was given is missing or malformed.

## Development mode

`-dev-no-auth` turns sign-in off for development. rigctlWeb refuses it unless it listens on a
loopback address, and logs a warning while it's on. Don't use it on a real station.

## Reporting a vulnerability

Please report security problems privately through GitHub: open the repository's **Security** tab
and choose **Report a vulnerability**. See
[SECURITY.md](https://github.com/open-rms/rigctlweb/blob/main/SECURITY.md).
