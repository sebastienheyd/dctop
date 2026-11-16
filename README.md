# dctop

dctop is a terminal dashboard for Docker, in the spirit of htop, organised by
Docker Compose project. One full-screen, keyboard-driven view shows every
Compose project known to the Docker daemon with its state and live CPU and
memory usage. You can start, stop, restart or remove a whole project, or
several at once, without going to the directory it was started from. Single
containers, volumes and images can be managed from the same screen, and
container logs and shells are one key away.

```
Projects   [|||||||||||||||||||||||||||    12/16] CPU [|                                       3.0%]
Containers [|||||||||||||||||||||          52/87] Mem [||||||||||                       3.60G/15.5G]
Volumes: 139   Images: 104                                                 dctop dev · Docker 29.7.2
 Projects   Volumes   Images
  NAME▼                                    STATE             CPU%     MEM INFO
▼ dctop-e2e                                running 2/2       0.0%   4.98M ~/projects/dctop-e2e
    dctop-e2e-web-1                        running           0.0%    440K Up 11 seconds
    dctop-e2e-worker-1                     running           0.0%   4.55M Up 11 seconds




Filter: dctop-e2e
F1Help F2Start F3Stop F4Filter F5Restart F6Sort F7Logs F8Shell F9Remove F10Quit
```

## Requirements

- Linux or macOS.
- The `docker` command line with the Compose v2 plugin (`docker compose`).
  dctop drives the `docker` command, so it works with whatever it is
  configured for: Docker contexts, `DOCKER_HOST`, rootless Docker, remote
  daemons over SSH.

## Install

With the install script, which downloads the latest release for your
platform, checks its SHA-256 checksum and installs it into `~/.local/bin`
(set `DCTOP_INSTALL_DIR` to choose another directory):

```sh
curl -fsSL https://raw.githubusercontent.com/sebastienheyd/dctop/main/install.sh | sh
```

With Go 1.26 or later:

```sh
go install github.com/sebastienheyd/dctop@latest
```

Or download an archive from the
[releases page](https://github.com/sebastienheyd/dctop/releases): static
binaries are provided for Linux and macOS, on amd64 and arm64.

## Usage

```sh
dctop                # start the dashboard
dctop --lang fr      # force the interface language
DCTOP_LANG=fr dctop  # same, through the environment
dctop --version      # print the version
```

### Projects

A project is the set of containers sharing a `com.docker.compose.project`
label. A project is *running* when all its containers run, *partial* when
some do, *stopped* when none does. Containers started without Compose are
grouped under "Standalone containers", at the end of the list.

Actions apply to the tagged rows shown in the current tab if there are any,
otherwise to the row under the cursor. Rows hidden by the filter, a collapsed
project or the hide-stopped toggle keep their tag but are left out until they
are shown again. At most four actions run at the same time; their result
appears on the status line.

| Target | Start | Stop | Restart | Remove |
| --- | --- | --- | --- | --- |
| Project | `docker compose -p NAME start` | `docker compose -p NAME stop` | `docker compose -p NAME restart` | `docker compose -p NAME down` |
| Container | `docker start` | `docker stop` | `docker restart` | `docker rm -f` |
| Volume | | | | `docker volume rm` |
| Image | | | | `docker image rm REPOSITORY:TAG` |

Removing always asks for confirmation, naming what will be removed.
Removing a project removes its containers and networks and keeps its
volumes. An image with several tags has one row per tag: removing a row
removes that tag, and the image goes with its last tag. Untagged images are
removed by ID.

### Keys

Every function key has a letter alias, since terminals often capture F1 and
F10.

| Key | Action |
| --- | --- |
| ↑ ↓ k j PgUp PgDn Home End | move |
| → ← Enter | expand / collapse the project under the cursor |
| Space | tag or untag the row and move down |
| U | untag everything |
| Tab, Shift+Tab, 1 2 3 | change tab (Projects, Volumes, Images) |
| F1 ? | help |
| F2 s | start |
| F3 x | stop |
| F4 / | filter by name |
| F5 r | restart |
| F6 o | cycle the sort key |
| F7 l | follow the logs of a project or a container |
| F8 e | open a shell in a running container |
| F9 d | remove (after confirmation) |
| H | hide or show stopped projects |
| F10 q Ctrl+C | quit |

In the logs view, scrolling up pauses following, `f` or End resumes it, and
Esc or `q` returns to the list.

## Language

The interface is available in English and French. The language is the first
usable value among `--lang`, `DCTOP_LANG`, `LANGUAGE`, `LC_ALL`,
`LC_MESSAGES` and `LANG`; `C` and `POSIX` are skipped, and English is used
when nothing else matches. Text produced by Docker itself is shown as Docker
writes it.

To add a language, copy `internal/i18n/locales/en.json` to
`internal/i18n/locales/<code>.json`, translate the values (keep the
`{placeholders}`), and run `go test ./internal/i18n/`, which checks that
every catalog has the same keys and placeholders.

## Development

```sh
make test    # go test ./...
make lint    # gofmt check and go vet
make build   # bin/dctop
```

## License

MIT, see [LICENSE](LICENSE).
