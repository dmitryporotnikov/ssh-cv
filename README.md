# ssh-cv

A tiny SSH server that shows off your CV or profile in the terminal. Connect via SSH, see a brief hacker-style animation, read your `info.md` rendered nicely, and you're done. No shell, no commands—just a clean read-only display.

## What It Does

- Listens for SSH connections on a configurable port
- Accepts anonymous access (no auth needed)
- Refuses anything that isn't a session channel
- Renders `info.md` with ANSI colors and basic markdown
- Keeps the same host key across restarts (generated on first run)
- Has handshake and session timeouts to prevent abuse
- Limits concurrent connections

## Files

- `info.md` — your markdown profile, rendered for connected users
- `config.yaml` — optional configuration
- `ssh_host_key` — generated on first run (unless you point `server.host_key_path` elsewhere)

Override paths with environment variables:

- `SSH_CV_CONFIG_PATH`
- `SSH_CV_INFO_PATH`
- `SSH_CV_HOST_KEY_PATH`

With Docker Compose, just edit the files in [runtime/](runtime/) on your host.

## Running

```bash
go run .
```

Defaults to port `2222`. Connect with any SSH client:

```bash
ssh -p 2222 any-user@host
```

It's presentation-only—no shell, no command execution, no SFTP, no port forwarding.

## Docker

### Docker Hub

Image: `dmitryporotnikov/ssh-cv:latest`

Run with defaults baked into the image:

```bash
docker run -d \
  --name ssh-cv \
  -p 2222:2222 \
  -v ssh-cv-data:/data \
  dmitryporotnikov/ssh-cv:latest
```

### Custom Content On The Host

Mount your own files instead of rebuilding:

```bash
mkdir -p "$HOME/ssh-cv-runtime"
```

Create `config.yaml` in that directory:

```yaml
server:
  port: 2222
  handshake_timeout_seconds: 10
  session_timeout_seconds: 300
  max_concurrent_connections: 100

messages:
  access_granted: ">> ACCESS GRANTED <<"
  press_to_exit: "[Press 'q' to disconnect]"
  goodbye: "Connection terminated."
  hack_message: "BYE!"
```

Create `info.md` in that directory:

```md
# Your Name

## About
Short intro here.

## Links
- GitHub: https://github.com/yourname
- Website: https://example.com
```

Run the container against those host files:

```bash
docker run -d \
  --name ssh-cv \
  -p 2222:2222 \
  -e SSH_CV_CONFIG_PATH=/runtime/config.yaml \
  -e SSH_CV_INFO_PATH=/runtime/info.md \
  -e SSH_CV_HOST_KEY_PATH=/data/ssh_host_key \
  -v "$HOME/ssh-cv-runtime:/runtime:ro" \
  -v ssh-cv-data:/data \
  dmitryporotnikov/ssh-cv:latest
```

In this setup:

- Your content lives on the host at `$HOME/ssh-cv-runtime`
- The container reads it at `/runtime/config.yaml` and `/runtime/info.md`
- The SSH host key stays in the Docker volume `ssh-cv-data`

Restart the container after editing your files:

```bash
docker restart ssh-cv
```

Note: env vars point to paths inside the container, not host paths.

### Build Locally

```bash
docker build -t ssh-cv .
```

### Compose

There's a [compose.yaml](compose.yaml) for local dev:

```bash
docker compose up --build
```

This mounts [runtime/](runtime/) read-only into the container. Edit your files on the host, restart to pick up changes.

## Configuration

Example `config.yaml`:

```yaml
server:
  port: 2222
  host_key_path: "ssh_host_key"
  handshake_timeout_seconds: 10
  session_timeout_seconds: 300
  max_concurrent_connections: 100

animations:
  matrix_lines:
    - "01001110 10101010 11110000 BINARY_INTRUSION_DETECTED"
    - "10110101 01010101 00110011 SYSTEM_HACK_V.3.7.1"
  matrix_speed_ms: 60
  scan_lines_count: 3
  scan_speed_ms: 5
  ascii_glitch: true
  boot_delay_ms: 100
  typewriter_speed_ms: 10
  show_scan_lines: true

messages:
  access_granted: ">> ACCESS GRANTED <<"
  press_to_exit: "[Press 'q' to disconnect]"
  goodbye: "Connection terminated."
  hack_message: "HACK THE PLANET!"
```

## Markdown Rendering

The renderer supports a lightweight subset of markdown:

- headings
- bold and italic text
- unordered and ordered lists
- inline code
- fenced code blocks with simple ANSI color themes for common languages
- links rendered as underlined text

## Security Model

No authentication—this is by design. Anyone who can reach the port can see your profile. That's the point. It's a kiosk, not a server for sensitive stuff.

Guardrails in place:

- Persistent host key on disk
- Handshake and session timeouts
- Connection cap
- Rejects unsupported channels and session request types
- Log sanitization for attacker-controlled SSH metadata

If you expose this publicly, treat it as anonymous read-only infrastructure and put it behind normal network controls for an internet-facing service.
