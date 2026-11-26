---
title: Install
description: Download rigctlWeb, set up rigctld for your radio, and run both as services on a Raspberry Pi or Linux.
order: 1
---

# Install

rigctlWeb is one program with its web page built in. It talks to your radio through Hamlib's
`rigctld`, so you need two things running: `rigctld` for the radio, and `rigctlweb`.

## 1. Install Hamlib

On Debian, Ubuntu and Raspberry Pi OS:

```sh
sudo apt install libhamlib-utils
rigctld --version        # Hamlib 4.6 or newer is recommended
```

On Fedora it's `sudo dnf install hamlib`. On macOS, `brew install hamlib`. On Windows,
install Hamlib from [its releases](https://github.com/Hamlib/Hamlib/releases).

## 2. Start rigctld for your radio

Find your radio's model number, then start `rigctld` on its serial or USB port:

```sh
rigctl -l | grep -i ftdx        # model numbers, e.g. 1042 for the Yaesu FT-DX10
rigctld -m 1042 -r /dev/ttyUSB0 -s 38400 -t 4532 -T 127.0.0.1
```

- `-m` is the model, `-r` the port, `-s` the baud rate (match the radio's CAT rate menu).
- `-t 4532` is the TCP port rigctlWeb connects to. `-T 127.0.0.1` keeps `rigctld` off the
  network: it has no password, so anyone who can reach its port can control the radio.
- Only `rigctld` should open the radio's port. Other programs (Pat, WSJT-X, fldigi) can share the
  radio by connecting to `rigctld` too. See [Integrations](integrations.md).

No radio handy? `rigctld -m 1 -t 4532` runs Hamlib's dummy radio, which is good for trying
rigctlWeb out.

Stable device names help when you have more than one USB serial device: use
`/dev/serial/by-id/...` instead of `/dev/ttyUSB0`.

## 3. Download rigctlWeb

Get the file for your computer from the [download table](https://rigctlweb.openrms.dev/#download)
or the [GitHub releases](https://github.com/open-rms/rigctlweb/releases). A Raspberry Pi with a
64-bit OS uses `linux-arm64`.

```sh
chmod +x rigctlweb-*-linux-arm64
mkdir -p ~/.local/bin
mv rigctlweb-*-linux-arm64 ~/.local/bin/rigctlweb
```

Each release has a `sha256sums.txt`; `sha256sum -c --ignore-missing sha256sums.txt` checks
your download.

## 4. First run

```sh
rigctlweb -rig 127.0.0.1:4532
```

On the first run it prints an **access token** and saves it in its config file
(`~/.config/rigctlweb/config.toml` on Linux). Open <http://127.0.0.1:8480>, sign in with the
token, and add your radio under **Settings → Radios & connection** so it is remembered. `-rig`
only adds a radio for that run.

By default rigctlWeb listens on your own computer only. To use it from a phone or another
computer on your network, start it with `-listen 0.0.0.0:8480` and read [Security](security.md)
first: it can key your transmitter.

Lost the token? Run `rigctlweb -show-token`, or look for `token` in the config file.

## Run it as a service (Linux, Raspberry Pi)

rigctlWeb runs well as a systemd **user** service. The repository has a unit file in
`packaging/systemd/rigctlweb.service`:

```sh
mkdir -p ~/.config/systemd/user
curl -fsSLo ~/.config/systemd/user/rigctlweb.service \
  https://raw.githubusercontent.com/open-rms/rigctlweb/main/packaging/systemd/rigctlweb.service
systemctl --user daemon-reload
systemctl --user enable --now rigctlweb
loginctl enable-linger "$USER"     # keep it running when you're logged out
journalctl --user -u rigctlweb -f  # logs
```

That unit listens on `0.0.0.0:8480` so other devices on your LAN can reach it. Edit
`ExecStart` if you want it on localhost only.

`rigctld` can run as a service too. A minimal user unit:

```ini
# ~/.config/systemd/user/rigctld.service
[Unit]
Description=Hamlib rigctld

[Service]
ExecStart=/usr/bin/rigctld -m 1042 -r /dev/serial/by-id/YOUR-RADIO -s 38400 -t 4532 -T 127.0.0.1
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
```

Your user needs access to the serial port: on Debian-based systems,
`sudo usermod -aG dialout "$USER"`, then log out and in again.

If you use [hamstack](https://github.com/ATCUSA/hamstack), it can run rigctlWeb for you behind
its own login. See the README's "Inside hamstack" section.

## Command-line options

| Option | What it does |
| --- | --- |
| `-listen ADDR` | Address and port to serve on (default `127.0.0.1:8480`). |
| `-rig HOST:PORT` | Add a rigctld for this run only. |
| `-config FILE` | Use another config file. |
| `-base-path /x` | Serve under a URL prefix behind your own reverse proxy. |
| `-show-token` | Print the access token at startup. |
| `-presets-url URL` | The openRMS Rigs API used by **Update library**. |
| `-debug` | Verbose logging. |
| `-version` | Print the version and exit. |

## Build from source

You need Go, Node and pnpm; the repository pins versions with [mise](https://mise.jdx.dev/).

```sh
git clone https://github.com/open-rms/rigctlweb.git
cd rigctlweb
mise install          # pinned Go, Node, pnpm
mise run setup        # web dependencies
mise run build        # bin/rigctlweb with the web page built in
```

`mise run dev` starts a dummy radio and the web page with hot reload.
