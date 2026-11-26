# rigctlWeb API

rigctlWeb publishes a small, versioned HTTP API (v1) that any program can use: a logger that fills in frequency and mode, a Home Assistant sensor, a script that changes band, a dashboard, or another web app. The API is described in [OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0), so you can generate a client for most languages.

- **Interactive reference:** open **API** from the Integrations page or search in rigctlWeb, or go to `/api-docs`. It is rendered by [Scalar](https://github.com/scalar/scalar) from the running server's own description, bundled into the binary, and works offline.
- **OpenAPI document:** `GET /api/v1/openapi.json` on a running server, or [`openapi.json`](openapi.json) in this folder. It is generated from the code (`rigctlweb openapi`), and a test keeps the committed copy current.

## Quick start

```sh
TOKEN=...   # from rigctlWeb's config file, or run it once with --show-token
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8480/api/v1/rigs
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8480/api/v1/rigs/ftdx10/state
curl -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"hz":14074000}' http://127.0.0.1:8480/api/v1/rigs/ftdx10/frequency
curl -N -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8480/api/v1/rigs/ftdx10/events   # live updates
```

With `--base-path /x`, every route lives under `/x` (for example `/x/api/v1/rigs`).

## Routes

| Route | What it does |
|---|---|
| `GET /api/health` | `200 {"status":"ok"}` when rigctlWeb can serve. No token. It does not depend on a radio being connected. |
| `GET /api/v1/info` | Program version and the API versions served. |
| `GET /api/v1/integration` | The handshake: mode, features, and each radio with whether it may transmit. |
| `GET /api/v1/openapi.json` | The OpenAPI document. |
| `GET /api/v1/rigs` | Radios: id, name, online, read-only. |
| `GET /api/v1/rigs/{id}` | One radio. |
| `GET /api/v1/rigs/{id}/state` | Frequency, mode, passband, VFO, split, RIT/XIT, whether it is transmitting, levels, functions and parameters. |
| `GET /api/v1/rigs/{id}/events` | The same state as [Server-Sent Events](https://html.spec.whatwg.org/multipage/server-sent-events.html): `event: state` right away and on every change, plus a `: ping` comment every 15 s. |
| `GET /api/v1/rigs/{id}/capabilities` | Model, modes, VFOs, levels (range, readable, settable), functions, RIT/XIT limits and split support. |
| `PUT /api/v1/rigs/{id}/frequency` | `{"hz":14074000}` |
| `PUT /api/v1/rigs/{id}/mode` | `{"mode":"USB","passbandHz":2400}`. Leave out `passbandHz` to keep the radio's default. |
| `PUT /api/v1/rigs/{id}/vfo` | `{"vfo":"VFOB"}` |
| `PUT /api/v1/rigs/{id}/split` | `{"enabled":true,"txVfo":"VFOB"}` |
| `PUT /api/v1/rigs/{id}/rit`, `/xit` | `{"hz":-150}` |
| `PUT /api/v1/rigs/{id}/levels/{name}` | `{"value":0.5}`, for a standard Hamlib level such as `AF`, `RF` or `RFPOWER`. |
| `PUT /api/v1/rigs/{id}/functions/{name}` | `{"enabled":true}`, for a receive-side or setup function such as `NB`, `NR`, `ANF` or `COMP`. Capabilities' `settable` flag says which ones. |

A successful `PUT` answers `200` with the radio's state after the change. Unknown fields are refused, so a typo gives an error instead of being silently ignored. A missing required field is refused too, so `{}` never quietly sets a level to 0 or switches something off.

## Transmitting is not part of v1

No v1 route keys the transmitter, starts a tune cycle, or turns on anything that makes the radio transmit by itself.

Functions are an allow-list of 36 receive-side and setup functions from Hamlib's `rig.h`: AGC, noise blankers and reduction, notch and filters, preamp, squelch, CTCSS and DCS settings, RIT/XIT, monitor, lock, mute, scan resume and the scope. Every other function answers `403`, and capabilities list it as not settable. That includes VOX, semi and full break-in, the 1750 Hz tone burst, and sending stored Morse or voice, all of which transmit. It also includes the automatic tuner (on some radios switching it on starts a tune cycle), `TRANSCEIVE` (it changes the radio's auto-reporting under rigctld), scrambling, and anything Hamlib adds in future. Allowing more later is a non-breaking addition.

The reason is safety on a shared radio. rigctlWeb's own UI keys only over a live connection, and if that connection drops rigctlWeb unkeys at once. A stateless HTTP request has no connection to watch. Keying may come later through a connection-bound API with the same limits (a 3-minute transmit timeout and unkey on disconnect). That needs its own design and the project owner's decision.

`transmitting` in the state does report whether the radio is keyed, by anyone.

## Authentication

Send the access token as `Authorization: Bearer <token>`. The web UI's session cookie also works, which is how the interactive reference's **Test Request** button works. In hamstack integration mode the token is off and hamstack's proxy secret is used instead (see below). Only `GET /api/health` needs nothing.

There is no CORS: a web page on another origin cannot call the API from a browser. Call it from a server or script, or serve your page from behind the same reverse proxy.

## Errors

Errors are [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details (`application/problem+json`):

```json
{ "type": "about:blank", "title": "Forbidden", "status": 403, "detail": "this radio is read-only" }
```

| Status | Meaning |
|---|---|
| 400 | Malformed request, or a value the radio does not accept (for example a mode it does not have) |
| 401 | No valid token |
| 403 | The radio is read-only, the function is not one v1 changes, or (hamstack mode) the proxy secret is missing |
| 404 | No radio with that id, or no such route |
| 405 | The route exists with another method (see the `Allow` header) |
| 422 | The radio or its Hamlib backend does not support this level or function |
| 502 | rigctld or the radio reported an error |
| 503 | The radio is not connected yet |
| 504 | The radio did not answer in time |

## Change policy

- The major version is in the path: `/api/v1/...`.
- Within v1, fields and routes are only added. Nothing is renamed or removed, and no field changes meaning. Tests pin every published route, schema and field name.
- Clients must ignore fields they do not know.
- A breaking change gets a new path, `/api/v2/...`. v1 keeps working alongside it for at least two minor releases, and the release notes say when it will be removed.
- `GET /api/health` follows the same rules, even though it has no version in its path.
- The OpenAPI document's `info.version` starts at `1.0.0`, and its minor number goes up with each addition.
- Any change to a published route is listed in the release notes.

Everything else (`/api/rigs`, `/api/views`, `/api/settings`, `/api/session`, `/api/login`, the audio, memory and presets routes, `/ws`, `/ws/audio`) is private to rigctlWeb's own web UI and can change in any release without notice. If you need something only a private route offers, please open an issue with the `integration` label: it will be published under `/api/v1` first.

## hamstack integration mode

Integration mode turns on when `HAMSTACK_INTEGRATION_ID` is set. hamstack provides:

| Input | Use |
|---|---|
| `-listen 127.0.0.1:PORT` | must be a loopback address, or rigctlWeb refuses to start |
| `-base-path /integration/<id>` (or `HAMSTACK_BASE_PATH`) | URL prefix for everything |
| `HAMSTACK_RIGS` | `id=host:port[,ro]` entries separated by `;`. These replace the config file's radios and `-rig`. Any malformed entry or unknown option stops start-up. |
| `HAMSTACK_PROXY_SECRET_FILE` | path of a file only its owner can read, holding at least 32 characters. Every request except `GET /api/health` must send it as `X-Hamstack-Proxy-Auth`, or it gets 403. |
| `HAMSTACK_CONFIG_DIR` | settings folder (`config.toml` for views and theme) |
| `HAMSTACK_DATA_DIR` | data folder. Keying markers go in `state/keyed-<rig id>`, where hamstack's stop hook looks for them. |

In this mode token sign-in is off (`POST /api/login` returns 404), and the rig list can't be changed (`POST` and `DELETE /api/rigs` return 403). `GET /api/session` reports `"mode":"hamstack"` so the UI hides sign-in, sign-out and the add and remove radio controls. Responses send `frame-ancestors 'self'` and `X-Frame-Options: SAMEORIGIN`. On SIGTERM it unkeys any radio it keyed first, then exits within 5 seconds.

`GET /api/v1/integration` is the handshake hamstack reads (hamgo and integrations design, section 4.5):

```json
{
  "name": "rigctlweb",
  "version": "1.2.3",
  "api": ["v1"],
  "mode": "hamstack",
  "capabilities": ["rig.control", "audio.monitor"],
  "transmits": true,
  "rigs": [{ "id": "ftdx10", "readOnly": false }]
}
```

`transmits` is true when at least one radio is not read-only. `capabilities` lists features this build has. A feature can be built in and still be off for a radio, as the audio monitor is by default.
