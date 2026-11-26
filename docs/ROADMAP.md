# rigctlWeb task list

The working list. `[x]` done, `[~]` in progress, `[ ]` to do. Newest ideas go in the section that fits; order inside a section is rough priority.

## Done

- [x] rigctld client (extended protocol), capability parsing from `dump_caps`, adaptive polling
- [x] Live UI: frequency, meters, modes, VFO/split/RIT/XIT, levels, functions, bands with per-band memory
- [x] Token auth, localhost-by-default, CSP, same-origin WebSocket, login rate limit, `--dev-no-auth` (loopback only)
- [x] Hard TX timeout, tune cooldown and 30 s carrier cap, live tune monitoring judged by final SWR
- [x] Dynamic panels from the radio's capabilities: filter width, tuning step, AGC/preamp/attenuator, FM tones and repeater, radio-specific extras, parameters
- [x] Parser checked against 44 radios; default ranges for range-less levels; fixtures for 7 real radios
- [x] Slow or failing background reads are benched so one stalled query can't block commands
- [x] Reconnect handling for rigctld's dropped-connection race
- [x] Instant UI feedback (confirmed changes shown immediately), coalesced frequency stepping, typed frequency entry (`F`)
- [x] Logo, favicon, app icons, manifest
- [x] Views: Essentials, Everyday, Advanced, Everything, plus custom views (copy, edit, reset)
- [x] Tooltips for levels, functions, parameters, modes and controls
- [x] Busy indicator (antenna) while changes apply
- [x] Responsive layout: one self-arranging column area (1 to 4 columns by width), collapsible panels with remembered state, sticky readout bar, two-row header on phones, touch-sized targets, no horizontal overflow checked from 360 px to 1920 px
- [x] Search (`/` or Ctrl+K) across every control, setting, view, theme and integration; jumps to the control, or adds it to the current view
- [x] Integrations page: per-program steps and copy-ready snippets with your rigctld host/port filled in, confidence and sources shown
- [x] Keying safety on a shared rigctld: rigctlWeb unkeys only what it keyed itself (tracked in memory plus a marker file), unkey-first shutdown within 5 s on a context SIGTERM can't cancel, unkey when the browser that keyed the radio disconnects, and crash recovery at start-up (fresh marker + radio still keyed). Tests prove it never cuts another program's transmission
- [x] Hamlib error codes fixed to match rig.h (the table was off by one from code 4 up; found by the hamstack session). `IsUnsupported` is now exactly: not implemented, not available, VFO not targetable, invalid parameter, invalid VFO, out of domain. A bus-busy or truncated reply no longer disables a control; a test checks every code against the installed rig.h
- [x] Public API v1: OpenAPI 3.1 generated from the route table and Go types (`docs/openapi.json`, `rigctlweb openapi`, `GET /api/v1/openapi.json`), interactive Scalar reference at `/api-docs` bundled in the binary (no CDN, telemetry and "Ask AI" off). Routes: health, info, integration handshake, rigs, state, Server-Sent Events, capabilities, and PUT frequency/mode/vfo/split/rit/xit/levels/functions (functions are an allow-list of 36 receive-side ones). RFC 9457 errors (also for unknown routes, 405 and the proxy 403), strict JSON with required fields, event streams end at shutdown. Checked in a browser behind a stand-in hamstack proxy (hidden controls, both WebSockets, SSE, Scalar Test Request). Tests pin names and routes, check the committed spec, validate it as OpenAPI 3.1, and run every route against the dummy rig. **No keying in v1** (PTT, tune, VOX, break-in, tone burst, Morse/voice send): owner to decide whether a connection-bound keying API comes later. Policy in `docs/API.md`
- [x] Base path (`--base-path`) and hamstack integration mode: proxy secret replaces token sign-in (403 without it, health excepted), radios from `HAMSTACK_RIGS` (fails closed on a malformed entry), settings and keying markers in hamstack's folders, localhost only, no token banner, frameable by hamstack, handshake reports `mode: "hamstack"`, exit within 5 s of SIGTERM. Checked in a browser under `/integration/rigctlweb` (reloads, assets, WebSocket, clean CSP) and with curl the way hamstack's prober calls it. Not yet run behind the real hamstack proxy
- [x] Packaging basics: deploy script, systemd user unit, CI, release workflow, issue/PR templates, donation statement

## Next up

- [ ] Verify the new layout on a real phone and tablet (checked so far with a headless browser at fixed sizes, not touch hardware)
- [ ] Let panels be reordered (drag or up/down) and remembered per view
- [ ] Keyboard-shortcut help overlay (`?`)
- [ ] Favorite radio models and a rigctld launcher/supervisor (pick model + serial port, start/stop rigctld)
- [~] Memory channels and scan, where the radio supports them. Phase 1 done: read channels on demand (`get_channel` with the read-only flag, checked on a real FT-DX10 to leave the radio untouched; one channel per 150 ms, at most 200 per read, paused while transmitting) and Tune, which copies frequency, mode, repeater shift/offset and tones to the current VFO through the normal validated commands (never memory mode). Only frequency banks are listed (MEM, CALL, EDGE, BAND, PRIO, MEMOPAD); VOICE and MORSE slots never are, because using them transmits. Private UI routes only (not in API v1 yet). Still to do: store to a memory (overwrites the radio's own memory: confirm with the old contents shown; wait for the owner to try phase 1 on a real radio), names, scan
- [ ] Band plan overlay: advisory only, never blocks transmit (EmComm, SHARES, MARS, CAP and commercial use must always work); region selectable
- [ ] Morse and voice-memory send (these key the transmitter: confirm + hard limits)
- [ ] Antenna selection, power on/off (with confirm), DCD/squelch indicator where available
- [~] Audio monitor (receive only, opt-in per rig, off by default): stream the radio's sound card to the browser. Waterfall/level meter are optional extras in the page, not required. Design: no cgo, so the single static binary still cross-compiles everywhere. The server runs a pluggable capture command that writes raw PCM to stdout (Linux: pw-record/parec/arecord or ffmpeg; macOS: ffmpeg avfoundation; Windows: ffmpeg wasapi/dshow), sends 16-bit mono PCM over the existing WebSocket, and the browser plays it with an AudioWorklet and draws any waterfall itself. Missing tools just disable the feature. Needs: device list per OS, channel pick (left/right/mix), retry on device loss. Checked on a dev Raspberry Pi (PipeWire, USB codec): shared capture works alongside pat and WSJT-X. Untested on macOS/Windows. Done so far: read-only `rigctlweb audio detect` (Linux PipeWire/Pulse/ALSA, macOS and Windows via ffmpeg; verified on arm64 and amd64 Linux). Receive-only listen is built and verified on a dev Pi: capture runs only while someone listens, mono mix/left/right, level meter with clip warning, volume per browser (audible output not verified by me; the meter and stream are). Clipping fix built: manual input slider, Restore original, and Auto-adjust (listens 2-5 s, lowers the input to a -6 dB peak, checks the level really fell, otherwise restores and points to the radio's output level). Input volume control works on PipeWire (wpctl), PulseAudio (pactl) and ALSA (amixer); macOS/Windows show manual advice only. Auto's adjust and 'did not respond' paths are unit-tested but not yet seen with a genuinely clipping signal. Still to do: optional waterfall, `setup`/`revert`, macOS/Windows testing. Setup helper: `rigctlweb audio detect|setup|revert` as Go subcommands (same on every OS, no grep/sed). PipeWire/Pulse/macOS/Windows need no config changes; only plain ALSA gets a dsnoop entry. Our changes live in our own drop-in file, or inside `# BEGIN rigctlweb` / `# END rigctlweb` markers in a shared file. Revert removes only that, never restores a whole-file backup (keeps user edits), warns if our block was edited, never touches mixer levels. Always `--dry-run` and `--print` first; user-level paths, sudo only if unavoidable.

## Later

- [x] openRMS Rigs radio presets (Presets page): browse presets like a manual (value, reason, sources, confidence, author credit), check locally against the radio, apply through rigctlWeb's own validated commands with transmit-side settings opt-in, restore from a saved snapshot. Library fetched only on "Update library" and cached for offline use. Whole preset refused on any keying or non-allow-listed function; raw CAT steps shown as manual steps (owner decision 2026-11-26), then the narrow raw-CAT path (below)
- [x] Narrow raw-CAT path for presets (owner decision 2026-11-26): openRMS `cat-allowlist.json` embedded and checked on every command (all 33 conformance vectors pass; denied prefixes win, longest family decides); ASCII CAT only (Kenwood, Yaesu, Elecraft) through rigctld `send_cmd` / `send_cmd_rx`; a step is sent only when its set command is its read command with one value, so the value is read before (for Restore) and read back after; the operator confirms the exact command strings and the server refuses if they differ; refused while transmitting or on read-only radios; every command (reads too) logged to `presets/cat-log.jsonl`, sets also to the journal; replies must be exactly the read command plus the value. Steps must pass the allow-list's `step_rules` (allow-list 1.1.0, all 19 step vectors pass) before anything is read: the read is the set template without its value, the reply is anchored to the read, and only meter-style families (set: false) may be read without a set. Raw CAT is not reachable from the WebSocket or API v1. Icom CI-V is validated but not sent
- [ ] Rigs presets follow-ups: sending Icom CI-V (needs framing with the radio's CI-V address, and a test radio); optional account features (favorites, "worked for me") via the OAuth device flow, never required
- [ ] rigctld proxy/multiplexer with per-client arbitration, so pat, wsjtx, fldigi, direwolf share one radio politely
- [ ] fldigi-compatible XML-RPC server
- [ ] PTT/modem coordination: who owns TX, PTT lock, per-modem profiles (Mercury, VARA-family, ARDOP, Packet)
- [ ] Spectrum/waterfall where the radio exposes scope data
- [ ] Station profiles ("FT8 20m", "Winlink 40m": frequency, mode, filter, power in one click)
- [ ] Log/ADIF assist: frequency, mode and band autofill for loggers
- [ ] Multi-rig dashboard and quick switching
- [ ] Notifications for high SWR, overtemperature, connection loss
- [ ] Prometheus / MQTT export (Home Assistant)
- [ ] Packaging: Windows service and installer, deb/rpm/AUR, macOS
- [ ] TLS option and passkey sign-in for LAN use
- [ ] Transmit audio from the browser mic (keys the transmitter: confirm + hard limits; needs HTTPS, so after TLS)

## Shared code plan (hamgo) and hamstack integration

Status: **design only, written by the hamstack session; nothing here is built, moved, tagged or pushed, and the owner has not yet approved creating the repo.** Full plan: `~/Projects/hamstack/docs/architecture/hamgo-and-integrations.md` (read-only for rigctlWeb sessions). Until the owner approves each step, rigctlWeb stays standalone and nothing in `internal/` moves.

What every agent must know (copied from section 8 of that document; rule 5 updated 2026-11-15 to the owner's decisions D5/D6: every feature a reusable package, hamstack's web UI on the same SvelteKit stack as rigctlWeb):

```
1. Shared code lives in ONE place: github.com/ATCUSA/hamgo (MIT, public, semver tags). Never copy a hamgo package into a program.
2. hamgo imports no host program, has no cgo, one dependency (coder/websocket, audiohttp only). Programs pin a hamgo TAG, never a branch.
3. Each component has ONE source of truth at any time (table in section 6). If it is not your repo today, do not edit it there.
4. rigctlWeb is an INTEGRATION (standalone app tied in through a versioned API), not a plugin. Plugins stay as specified in docs/plugins/.
5. Every feature is a reusable package (default home: hamgo); the CLI, TUI and web are thin front ends over them. The web UI is SvelteKit + TypeScript + Tailwind + shadcn-svelte, like rigctlWeb. Do not re-implement rigctlWeb panels in hamstack.
6. Nothing transmits without an explicit user action; every keyer has a hard limit and unkeys on exit, but ONLY if it keyed (rigctld is shared: a blind unkey cuts another program's over); the audio monitor never stops a modem or app.
7. System-wide changes (input volume is one) are explicit user actions, remembered so they can be restored, and ledger-recorded in hamstack.
8. Stop and ask the owner before: creating or pushing a GitHub repo, tagging, changing a module path, merging to main, anything on the Pi (for hamstack and plugin agents; rigctlWeb sessions deploy to the dev Pi under their own setup).
9. Tests use fakes for exec, files and rigctld, and fixtures captured from real systems. Never a real sound card, rigctld or radio in a test.
10. Secrets never go in URLs, command lines, environment values or logs. Pass a file path; the file is mode 0600.
```

rigctlWeb-specific notes (these differ from, or add to, the box):

- Rule 8 "anything on the Pi": rigctlWeb sessions use the dev Pi for development under the owner's standing dev access. Anything that could transmit still needs the owner's explicit OK.
- Rule 6 and "unkey on exit/start-up": today `Rig.Unkey` sends `set_ptt 0` unconditionally, and rigctld is shared with pat and others. An unkey at start-up, or when the last browser leaves, must only fire if rigctlWeb itself keyed (remember it, e.g. a small marker file cleared on unkey), or it could cut another program's transmission. The hamstack document now adopts this (a persisted marker file, cleared on unkey, covers start-up, shutdown, last controller leaving and crash recovery); rigctlWeb implements it in step 8.
- The rigctl client's retry policy (never retried: `vfo_op`, `send_*`, `scan`, `reset`, `power*`, `stop_`, `wait_`) is a safety property; it moves to hamgo with its tests.
- Audio is currently committed and quiet (no in-progress audio work). From the "audio move day" `internal/audio` becomes read-only here; new audio features are then written in hamgo. Not before the owner approves.
- The poller, optimistic state, benching and TX safety (3 min TX timeout, tune cooldown, 30 s carrier cap) stay rigctlWeb-owned.
- The integration-mode work (base path, proxy secret, `HAMSTACK_RIGS`, frame headers, `/api/v1/integration`) touches `cmd/`, `internal/server` and `web/` only, so it can proceed in parallel with the hamgo moves. Base path in a static SvelteKit SPA needs relative asset paths and all `/api`, `/ws`, `/ws/audio` URLs derived from the page's base, so test it early.
- Mic transmit audio (Later list) overlaps hamstack's voice plugin; do not build it until the owner decides which one carries it.

Step 8 (unkey-first shutdown with the "only if we keyed" rule) is done in rigctlWeb: `internal/rig/keying.go`, hamstack's marker-file wording matches. Marker lives in `<config dir>/state/keyed-<rig id>` (in integration mode `$HAMSTACK_DATA_DIR/state/`, step 15 done); markers older than 15 min are ignored at start-up.

What hamstack's reverse proxy does (reported by the hamstack session; local only, in `~/Projects/hamstack2`, `internal/web/proxy.go` and `docs/api/v1.md`). Integration mode can rely on it:

- Mounted at `/integration/<id>/` behind hamstack's login. The path and query are forwarded unchanged, so rigctlWeb must honour a base path itself.
- Adds `X-Hamstack-Proxy-Auth: <secret>` to every forwarded request, including WebSocket upgrades. The secret is read from a 0600 file per integration. Any browser-sent copy of that header, `X-Hamstack` and hamstack's session cookie are stripped; rigctlWeb's own cookies pass through.
- Preserves `Host` and `Origin`. Cross-origin POST and WebSocket requests are refused before they reach rigctlWeb.
- Adds `frame-ancestors 'self'` to rigctlWeb's CSP (appends it if rigctlWeb sends a CSP without it) and turns `X-Frame-Options: DENY` into `SAMEORIGIN`.
- 502 when rigctlWeb is down or has no secret, 404 when the integration is not running.

Steps affecting rigctlWeb (numbers from the hamstack document, section 6.2): 5 switch to `hamgo/rigctl`; 8 unkey-first shutdown (independent, useful standalone); 9-10 audio move day; 12 `audiohttp` handlers; 13 vendored browser module with a drift test; 15 integration mode.

## Housekeeping

- [ ] Confirm donation handles in `FUNDING.yml` and `docs/DONATING.md` (currently `ATCUSA`)
- [ ] Rotate the dev Pi's access token (it was logged once before the print-once fix)
- [ ] Remove the dev SSH key from the dev Pi when development access is no longer needed
- [x] Create the public GitHub repo (`open-rms/rigctlweb`, history squashed to one commit), enable private vulnerability reporting, create `radio-support` and `integration` issue labels
