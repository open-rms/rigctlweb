<p align="center"><img src="docs/logo.png" alt="rigctlWeb" width="520"></p>

# rigctlWeb

A live, browser-based radio control panel for [Hamlib](https://hamlib.github.io/)'s `rigctld`, in the spirit of flrig but built for the web: big tuning display, real-time meters, every control your radio advertises, and a clean settings page. One static binary, runs on Linux (first-class), Windows and macOS.

> Status: early development. The core control surface works; see the roadmap.

## Quick start

```sh
mise install          # pinned Go, Node, pnpm
mise run setup        # web dependencies
mise run build        # -> bin/rigctlweb (UI embedded)

rigctld -m 1 -t 4532 &                 # Hamlib dummy rig, or your radio's model and port
./bin/rigctlweb -rig 127.0.0.1:4532    # prints an access token on first run only (--show-token to print it again)
```

Open `http://127.0.0.1:8480` and sign in with the token. Add more radios under **Settings > Radios**.

Development with hot reload and a dummy rig: `mise run dev` (UI on `:5173`).

## What works today

- Frequency display you can scroll, click, or type into; mode, VFO A/B, split, RIT/XIT, band buttons with per-band memory
- Controls are built from `\dump_caps`, so each radio shows only what it supports (levels, functions, modes, VFOs)
- Live S-meter and TX meters (power, SWR, ALC, compression, temperature) with peak hold
- Multiple radios, auto-reconnect, hard TX timeout (default 3 min) that unkeys the radio
- Radio presets from [openRMS Rigs](https://rigs.openrms.dev): settings shared by other operators for your model and an operation (FT8, Winlink, SSB…). Read each like a manual with its sources, check it against your radio, apply what rigctlWeb can set safely, and restore your old values. The library downloads only when you press "Update library" and then works offline. Presets never key the transmitter. Menu settings that need raw CAT (Kenwood, Yaesu and Elecraft) are sent only if they're on openRMS's allow-list and can be read back, after you confirm the exact commands. A read command is sent only when it is that setting's set command without the value, so it can't change anything. Every raw command sent, reads included, is logged. Anything else is shown for you to set on the radio
- Memory channels: read them without changing the radio, and tune the VFO to one
- Light, dark and red night themes; collapsible settings sections
- Token auth, same-origin WebSocket, CSP, per-IP login rate limit

## Security

Listens on `127.0.0.1:8480` by default. To reach it from another machine use `--listen 0.0.0.0:8480`, keep the token secret, and put it behind TLS (Caddy or nginx) or a VPN (WireGuard, Tailscale). It can key your transmitter, so never expose it directly to the internet. See [SECURITY.md](SECURITY.md).

To serve it under a path behind your own reverse proxy, add `--base-path /rigctlweb`. The UI, API and WebSockets then all live under `/rigctlweb/`.

### Inside hamstack

rigctlWeb can also run as a [hamstack](https://github.com/ATCUSA/hamstack) integration. hamstack starts it with `-listen 127.0.0.1:PORT -base-path /integration/rigctlweb` and a few `HAMSTACK_*` variables, then shows it behind hamstack's own login. In that mode rigctlWeb:

- answers only requests that carry hamstack's proxy secret (health excepted), and turns its own token sign-in off
- uses the radios hamstack gives it, read-only unless you allowed transmitting in hamstack
- listens on localhost only, prints no token, and lets hamstack's pages frame it

It refuses to start if anything it was given is missing or malformed. Details are in [docs/API.md](docs/API.md).

## API

rigctlWeb has a published, versioned REST API (v1) for your own programs, scripts and dashboards. You can list radios, read their state, stream changes as Server-Sent Events, and set frequency, mode, VFO, split, RIT/XIT, levels and functions. It is described in OpenAPI 3.1 ([docs/openapi.json](docs/openapi.json)), with an interactive reference built into the app at `/api-docs`. v1 never keys the transmitter. Other routes are private to the web UI and may change. See [docs/API.md](docs/API.md).

## Roadmap

- Band-plan overlay (advisory only; EmComm, SHARES, MARS, CAP and commercial use are never blocked)
- Favorite radio models, rigctld launcher and supervisor
- Storing memories, station profiles (one-click "FT8 20m"), scan lists
- rigctld proxy/multiplexer, fldigi-compatible XML-RPC, config snippets for pat, fldigi, wsjtx-improved, direwolf, xastir, js8call and modems
- Spectrum/waterfall where the rig supports it
- Packaging (Windows service, deb/rpm/AUR, macOS)
- Prometheus/MQTT export

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Radio quirks and `dump_caps` reports are especially welcome.

## Free as in ham radio

No license keys, no paywalls, no kill switches, ever, and competing or interoperating projects are welcome. If it's useful to you, a one-time or recurring tip is appreciated but never required. See [docs/DONATING.md](docs/DONATING.md).

- [Buy me a coffee](https://buymeacoffee.com/ATCUSA)
- [Ko-fi](https://ko-fi.com/ATCUSA)

## License

MIT. See [LICENSE](LICENSE).
