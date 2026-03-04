# pinentry-go

A Wayland-native GTK4 GUI replacement for
[pinentry](https://gnupg.org/software/pinentry/index.html) that shows a clear
visual indicator of **which key is being unlocked**.

When you have multiple GPG/SSH keys it can be hard to tell which passphrase
you are being asked for. `pinentry-go` solves this by letting you assign a
name and an accent color to each key. The dialog header is highlighted with
that color so you always know which key is at stake at a glance.

![Screenshot placeholder](docs/screenshot.png)

## Features

- Native Wayland support (GTK4, no XWayland required)
- Per-key accent color and name configured via a simple TOML file
- Prefix-based key matching (one rule can cover a whole class of keys)
- Drop-in replacement for any `pinentry` variant — speaks the standard Assuan
  protocol over stdin/stdout
- Config is reloaded on every dialog open; no restart needed

## Installation

### From source

Build dependencies: `libgtk-4-dev`, `libglib2.0-dev`, `libgirepository1.0-dev`, `gcc`

On Fedora/RHEL: `gtk4-devel glib2-devel gobject-introspection-devel gcc`
On Debian/Ubuntu: `libgtk-4-dev libglib2.0-dev libgirepository1.0-dev gcc`

```sh
go install github.com/stefanv/pinentry-go/cmd/pinentry-go@latest
```

Or clone and build manually:

```sh
git clone https://github.com/stefanv/pinentry-go
cd pinentry-go
go build -o pinentry-go ./cmd/pinentry-go
sudo install -m 755 pinentry-go /usr/local/bin/
```

## Configuration

### Tell gpg-agent to use pinentry-go

Add to `~/.gnupg/gpg-agent.conf`:

```
pinentry-program /usr/local/bin/pinentry-go
```

Then restart the agent:

```sh
gpgconf --kill gpg-agent
```

### Key color configuration

Create `~/.config/pinentry-go/config.toml`:

```toml
# Accent color shown for keys that don't match any rule below.
[defaults]
color = "#888888"
name  = "Unknown key"

# Rules are matched in order; the first matching rule wins.
# 'match' is a substring match against the key identifier that gpg-agent
# provides.  To find the identifier for a key, run:
#
#   gpg --list-keys --with-keygrip
#
# The identifier has the form:
#   n/HEXSTRING  — encryption subkey
#   s/HEXSTRING  — signing subkey
#   u/HEXSTRING  — authentication subkey (SSH)
#
# You can match by hex ID alone (no type prefix needed), or match a whole
# class by using just the prefix, e.g. "u/" for all SSH keys.

[[keys]]
match = "AABBCCDD"            # matches any key whose ID contains this hex
name  = "SSH Key (work)"
color = "#0066cc"             # blue

[[keys]]
match = "11223344"
name  = "Unix Pass store"
color = "#cc0000"             # red

[[keys]]
match = "s/"                  # matches all signing keys
name  = "Signing key"
color = "#007700"             # green
```

The config file is optional; without it every dialog shows the default gray
accent.

## Assuan protocol support

| Command       | Supported |
|---------------|-----------|
| GETPIN        | ✓         |
| CONFIRM       | ✓         |
| MESSAGE       | ✓         |
| SETDESC       | ✓         |
| SETPROMPT     | ✓         |
| SETTITLE      | ✓         |
| SETOK         | ✓         |
| SETCANCEL     | ✓         |
| SETNOTOK      | ✓         |
| SETERROR      | ✓         |
| SETKEYINFO    | ✓         |
| SETREPEAT     | ✓         |
| SETTIMEOUT    | ✓         |
| SETQUALITYBAR | planned   |
| SETGENPIN     | planned   |

## License

MIT — see [LICENSE](LICENSE).
