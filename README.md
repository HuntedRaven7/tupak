# tupak
A Go Charm stack TUI for flatpaks — Bazaar-like browsing in the terminal.

## Run

```sh
go run ./cmd/tupak
# or
go build -o tupak ./cmd/tupak && ./tupak
```

Requires `flatpak` on PATH and Go 1.25+ (`charm.land/bubbletea/v2`).

## What it does (v1)

- **Remotes**: lists `flatpak remotes`, add/remove remotes, per-action `--user`/`--system` picker.
- **Flathub prompt**: on startup, if no `flathub` remote exists, a modal asks to add Flathub's remote.
- **Categories**: hardcoded Bazaar-style list (Audio & Video, Games, Office, …), apps-only browsing via Flathub `/api/v2/appstream` (24h cache in `~/.cache/tupak/`), offline fallback to `flatpak remote-ls`/`search`.
- **Search**: Flathub search with `tab` toggle for **Apps / Runtimes / Both** (browse stays apps-only per design).
- **Details**: name, developer, license, version, homepage, description — no screenshots.
- **Installed**: `flatpak list --app`, uninstall/update with scope picker.

## Keys

`1` home • `2` search • `3` installed • `4` remotes • `/` search • `tab` type toggle • `i` install • `x/d` remove • `u/U` update • `a` add remote • `?` help • `q` quit
