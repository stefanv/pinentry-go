# pinentry-go

## IMPORTANT NOTE

This project was built as an experiment, using Claude. It
solves a real-world problem for me, which is that I have several types
of keys managed by GPG keyring. Sometimes, git requires both my
signing key and my SSH key, and it's not always apparent which one I
am unlocking. I therefore wanted a pinentry program that very clearly
visually distinguishes between keys.

You are welcome to use this code, but take it for what it is.
I'd probably be a bit nervous about running an unverified pinentry program written by someone else—so why not take a look at the source and verify for yourself first (there's not that much of it)?
For an extra layer of safety, you can build the binary yourself too:

```
go build -o /tmp/pinentry-go ./cmd/pinentry-go
```

## Introduction

A Wayland-native GTK4 GUI replacement for
[pinentry](https://gnupg.org/software/pinentry/index.html) that shows a clear
visual indicator of **which key is being unlocked**.

When you have multiple GPG/SSH keys it can be hard to tell which passphrase
you are being asked for. `pinentry-go` solves this by letting you assign a
name and an accent color to each key. The dialog header is highlighted with
that color so you always know which key is at stake at a glance.

![Screenshot placeholder](docs/screenshot.png)

### What is a keygrip?

A keygrip is gpg-agent's internal identifier for a key — a fixed-length hex
string that uniquely identifies a key regardless of its format or storage
location. It is what gpg-agent uses when it asks pinentry-go which key is
being unlocked.

For unrecognized keys, pinentry-go displays the keygrip in the dialog header
so you can copy it directly into your config. To look up keygrips in advance:

```sh
gpg --list-secret-keys --with-keygrip   # GPG keys and GPG-managed SSH auth keys
gpg-connect-agent "keyinfo --list" /bye  # all keys the agent holds, including SSH
```

For SSH keys added via `ssh-add`, there is no direct keygrip-to-filename
mapping. The most reliable method is to correlate by position: the *n*th entry
in `gpg-connect-agent "keyinfo --list" /bye` corresponds to the *n*th entry
in `ssh-add -L`, which shows the public key and its comment (usually the
original filename).

## Features

- Native Wayland support (GTK4, no XWayland required)
- Per-key accent color and name configured via a simple TOML file
- Per-key matching via hex keygrip substring
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
# 'match' is a substring match against the SETKEYINFO value that gpg-agent
# sends.  The value has the form "<status>/<hexkeygrip>" where <status> is
# a cache-state letter (n=not cached, s=session cache, t=TTL expired,
# u=in use).  Match on the hex keygrip.
#
# Find keygrips with:
#   gpg --list-keys --with-keygrip          (GPG keys)
#   gpg-connect-agent "keyinfo --list" /bye  (all keys including SSH)

[[keys]]
match = "AABBCCDD"            # hex keygrip substring
name  = "SSH Key (work)"
color = "#0066cc"             # blue

[[keys]]
match = "11223344"
name  = "Unix Pass store"
color = "#cc0000"             # red

[[keys]]
match = "EEFF0011"
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
