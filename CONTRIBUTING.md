# Contributing to rigctlWeb

Thanks for helping. rigctlWeb is a single-binary web app (Go backend, Svelte frontend) that controls radios through Hamlib's `rigctld`. Bug reports, radio notes, and patches are all welcome.

Please read the [Code of Conduct](CODE_OF_CONDUCT.md) first. Security problems go through [SECURITY.md](SECURITY.md), not public issues.

## Development setup

Tool versions are pinned in `mise.toml` (Go, Node, pnpm).

```sh
git clone https://github.com/open-rms/rigctlweb.git
cd rigctlweb
mise install
pnpm -C web install
mise run dev
```

Common tasks:

| Task | Command |
| --- | --- |
| Run backend and frontend in dev mode | `mise run dev` |
| Production build (embeds the UI) | `mise run build` |
| Go and frontend tests | `mise run test` |
| Linters | `mise run lint` |
| Type and Svelte checks | `mise run check` |

You also need Hamlib installed so `rigctld` is on your PATH (`libhamlib-utils` on Debian and Ubuntu, `hamlib` on Arch and Homebrew).

## Testing without a radio

Hamlib ships a dummy rig. Start it and point rigctlWeb at it:

```sh
rigctld -m 1 -t 4540
```

Most work, including frequency, mode, and PTT handling, can be tested this way. Use real hardware for anything radio-specific, and say which radio in your PR.

## Code conventions

- Go: standard `gofmt`, `go vet` clean, table-driven tests where practical. Keep packages small and avoid global state.
- Frontend: Svelte 5 (runes), TypeScript, Tailwind CSS v4, and shadcn-svelte components. Prefer adding or composing shadcn-svelte components over writing one-off styled elements.
- UI must work on a phone-sized screen and with keyboard navigation.
- Keep dependencies few. Justify new ones in the PR.

## Commits and branches

Use [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `ci:`. Use a scope when helpful, for example `fix(rig): handle empty mode reply`.

Branch names: `feat/short-name`, `fix/short-name`, `docs/short-name`, `radio/model-name`.

## Pull request process

1. Open or find an issue first for anything larger than a small fix.
2. Branch from `main` and keep the PR focused.
3. Run `mise run lint`, `mise run check`, and `mise run test` locally.
4. Fill in the PR template, including how and on which rig you tested. Add screenshots for UI changes.
5. A maintainer will review. Expect requests for changes; squash merges are the default.

## Adding support notes for a radio

Radio-specific behavior lives in the docs so other owners can find it.

1. Open a "Radio support" issue with the model, the Hamlib model number, and the output of `rigctl -m <n> -r <dev> \dump_caps`.
2. Add or update the radio's entry under `docs/radios/` with: make and model, Hamlib model number, rigctld command line that works, firmware tested, what works, known gaps, and any menu or CAT settings required.
3. If code changes are needed (for example a quirk for a missing level), keep them isolated and covered by a test using recorded rigctld replies.

## Safety rules

These apply to every change.

- Band-plan information is advisory only. Never block or disable transmit because of a band-plan warning. Emergency communications, SHARES, MARS, CAP, and commercial or other licensed operation must all work.
- PTT must fail safe. Release PTT on disconnect, on backend error, on client timeout, and on shutdown. Never leave a radio keyed because of a bug or a dropped connection.
- Do not add behavior that silently transmits. Every transmit action must come from an explicit user action or a clearly configured integration.
- Never commit secrets, tokens, or personal data. Redact logs before posting them.
