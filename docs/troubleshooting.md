---
title: Troubleshooting
description: Not connecting, missing controls, slow radios, sign-in, the audio monitor and presets.
order: 6
---

# Troubleshooting

First, the logs: `journalctl --user -u rigctlweb -f` when it runs as a service, or the terminal you
started it from. Add `-debug` for more detail.

## The header says "Not connected"

rigctlWeb can't talk to `rigctld`.

- Is `rigctld` running, on the host and port you entered? Test it directly:
  `rigctl -m 2 -r 127.0.0.1:4532 f` should print the frequency.
- If `rigctl` can't reach the radio either, the problem is between `rigctld` and the radio:
  - The model (`-m`) or baud rate (`-s`) doesn't match the radio's CAT settings.
  - Your user can't open the serial port: add yourself to `dialout` (Debian-based systems) and
    log in again.
  - Another program has the serial port open. Only `rigctld` should; point other programs at
    `rigctld` instead.
- rigctlWeb retries on its own, so once `rigctld` works, the panel connects within a few seconds.

## A control is missing or wrong

The panel shows what `rigctld` says the radio supports. Check that view of it with
`rigctl -m 2 -r 127.0.0.1:4532 \dump_caps`. Try **Everything** under views, too: the smaller views
hide some controls on purpose. If Hamlib gets it wrong, it's a Hamlib backend matter; if Hamlib is
right and rigctlWeb is wrong, please open an issue with the `radio-support` label and the
`dump_caps` output.

## "slow rigctld command" in the log

Some radios take a second or more to answer certain reads, especially over a busy USB link or when
several programs poll at once. rigctlWeb pauses a background read that keeps being slow so your
own commands don't wait behind it. Reduce polling in other programs sharing the radio if it
persists.

## I lost the token

Run `rigctlweb -show-token`, or open the config file (`~/.config/rigctlweb/config.toml` on Linux)
and look for `token`.

## A phone or other computer can't reach it

- rigctlWeb listens on localhost only by default. Start it with `-listen 0.0.0.0:8480`.
- Check the computer's firewall allows TCP 8480 from your LAN.
- Behind a reverse proxy, the proxy must pass WebSocket upgrades through, and `-base-path` must
  match the path you proxy.

## The audio monitor is silent

- Turn it on for the radio, then pick the **Sound input**: the radio's USB audio codec, not the
  computer's microphone.
- Some radios send receive audio on the left channel only: set **Channel** to Left.
- macOS and Windows need `ffmpeg` installed and on the PATH.
- If the input works but is very quiet or distorted, adjust the input level; you can put the
  original level back.

## Presets

- **The list is empty**: press **Update library**. rigctlWeb doesn't download presets until you
  ask.
- **"No presets for your radio yet"**: switch the list to all radios to read the others, or write
  one on [openRMS Rigs](https://rigs.openrms.dev).
- **A setting says "set it on the radio"**: the reason is shown under it. Common ones: the radio
  doesn't use Kenwood/Yaesu-style CAT, the step has no read command (so it couldn't be restored),
  or the radio can't set that control through Hamlib.
- **Check shows "unknown"**: the radio didn't answer that read; hover over it for the error. Some
  radios refuse certain reads.
- **A raw CAT step failed with "reads back"**: the radio didn't take the value. Restore still
  covers it. Check the radio's menu, and the CAT log in `presets/cat-log.jsonl` under the state
  folder.

## Still stuck?

Ask on [GitHub Discussions](https://github.com/open-rms/rigctlweb/discussions), or open an issue
with your rigctlWeb and Hamlib versions, your radio and its Hamlib model number, and the log.
