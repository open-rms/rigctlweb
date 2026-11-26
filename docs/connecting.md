---
title: Connecting a radio
description: Add a radio, use more than one, make one read-only, and share a radio with other programs.
order: 2
---

# Connecting a radio

rigctlWeb never opens your radio's serial port. It connects to `rigctld` over TCP, and `rigctld`
talks to the radio. That keeps the radio-specific work in Hamlib, and lets other programs use the
same radio at the same time.

## Add a radio

Open **Settings → Radios & connection** and fill in:

- **Name**: what the panel shows, e.g. "FT-DX10".
- **rigctld host:port**: where `rigctld` listens, usually `127.0.0.1:4532`.
- **Read-only**: monitor only. rigctlWeb shows the radio's state but never changes it, never
  sends raw CAT, and never keys it.

Press **Add**. The header shows **Connected** once rigctlWeb has read the radio's capabilities.
If `rigctld` stops or restarts, rigctlWeb keeps retrying and reconnects on its own.

## What the panel shows

When it connects, rigctlWeb asks `rigctld` what the radio supports (`dump_caps`) and builds the
panel from that: its modes, VFOs, levels, functions and settings. A control your radio doesn't
have isn't shown, so two radios can look quite different.

Hamlib's support varies by radio and backend. If a control is missing or misbehaves, check it
with `rigctl -m 2 -r 127.0.0.1:4532` first; if it's wrong there, it's a Hamlib matter. Reports of
radio quirks are welcome as GitHub issues with the `radio-support` label, ideally with the output
of `rigctl -m 2 -r 127.0.0.1:4532 \dump_caps`.

## More than one radio

Add each `rigctld` as its own radio; each needs its own `rigctld` on its own port. Switch radios
from the header.

## Slow radios

Some radios take seconds to answer certain questions. rigctlWeb reads the important things
(frequency, mode, transmit state, meters) often and the rest in the background. A background read
that is slow or keeps failing is paused for a while so it doesn't hold up your commands. The log
says when that happens.

## Sharing the radio with other programs

`rigctld` accepts several clients, so Pat, WSJT-X, fldigi, JS8Call and rigctlWeb can share one
radio. Point each at the same host and port. A few rules keep it smooth:

- Let one program own transmit (PTT).
- Keep polling light: once a second or slower in other programs.
- Don't give the serial port to anything but `rigctld`.

The **Integrations** page in rigctlWeb has setup steps for common programs, with your own host and
port filled in. The same notes are in [Integrations](integrations.md).

## Behind a reverse proxy

To serve rigctlWeb under a path, for example `https://station.example/rigctlweb/`, start it with
`-base-path /rigctlweb` and proxy that path to it. The page, the API and the live updates all move
under the prefix. Your proxy must pass WebSocket upgrades through. See [Security](security.md) for
TLS.
