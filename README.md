# Pine

Pine is a Go daemon that manages tree services (long-running processes). It watches a configuration directory for `.tree` files and automatically manages the lifecycle of configured services.

## Features

- **Config-driven service management** - Define services in simple `.tree` config files
- **Hot config reload** - Automatically reloads and restarts services when config files change
- **Log rotation** - Automatic log rotation with age-based cleanup
- **User privilege separation** - Run services as different users
- **HTTP API** - Control trees via Unix socket HTTP endpoints
- **Graceful shutdown** - Handles SIGINT and SIGTERM for clean shutdowns

## Configuration

Tree services are configured using `.tree` files in the config directory. Each file contains key-value pairs (one per line).

### Config Options

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `Name` | No | filename | Service name |
| `Command` | Yes | - | Command to execute |
| `User` | No | "op" | Run as user |
| `EnvironmentFile` | No | - | Path to environment variables file |
| `LogFile` | No | `/var/log/homelab/{name}.log` | Path to log file |
| `MaxLogAge` | No | 7 | Days to retain logs |
| `Restart` | No | "never" | always, never, or limited |
| `RestartAttempts` | No | 3 | Max restart attempts (limited mode) |
| `RestartDelay` | No | 3s | Delay between restarts |

### Example Config

```ini
# myservice.tree
Name        myservice
Command     /usr/bin/myservice --config /etc/myservice.conf
User        myuser
EnvironmentFile /etc/myservice.env
LogFile     /var/log/myservice.log
MaxLogAge   14
Restart     always
RestartDelay 5s
```

## CLI Flags

```
-d  Directory to find .tree config files (default: /usr/local/etc/forest.d)
-e  Unix socket endpoint for HTTP API (default: /var/run/pine.sock)
-unprivileged  Run as the current user instead of root
```

## HTTP API

Pine exposes a HTTP API over a Unix domain socket for controlling trees.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/tree/start/{treeName}` | Start a tree |
| POST | `/tree/stop/{treeName}` | Stop a tree |
| POST | `/tree/restart/{treeName}` | Restart a tree |
| POST | `/tree/logrotate/{treeName}` | Rotate tree's log file |
| GET | `/tree/{treeName}` | Get tree status |
| GET | `/tree` | List all trees |

### Response Format

```json
{
  "name": "myservice",
  "status": "running",
  "lastChange": 1704067200,
  "uptime": 3600
}
```

### Client Library

The `arborist` package can be used programmatically:

```go
client := arborist.NewClient("/var/run/pine.sock")
err := client.StartTree(ctx, "myservice")
```

## Building

```bash
# Build pine daemon
go build -o pine

# Build arborist CLI client
go build -o arborist ./cmd/arborist
```

## Running

```bash
# Run with default config directory
sudo ./pine

# Run with custom config directory
sudo ./pine -d /etc/pine.d

# Run in unprivileged mode (for development)
./pine -unprivileged
```

## Arborist CLI

Arborist is a command-line client for controlling Pine trees via the daemon's HTTP API.

### CLI Flags

```
-e  Pine daemon endpoint (default: /var/run/pine.sock)
-t  Command timeout (default: 10s)
```

### Commands

| Command | Args | Description |
|---------|------|-------------|
| `start` | `<treeName>` | Start a tree |
| `stop` | `<treeName>` | Stop a tree |
| `restart` | `<treeName>` | Restart a tree |
| `status` | `<treeName>` | Get tree status |
| `list` | - | List all trees |
| `logrotate` | `<treeName>` | Rotate tree's log file |

### Examples

```bash
# Start a tree
./arborist start myservice

# Stop a tree
./arborist stop myservice

# Get status
./arborist status myservice

# List all trees
./arborist list

# Custom endpoint
./arborist -e /tmp/pine.sock status myservice
```

## Architecture

```
pkg/
  api/       - HTTP API types
  arborist/  - Client library for Pine API
  pine/      - Daemon implementation
  tree/      - Tree service management
```

- **Daemon** (`pkg/pine/`) - Main daemon that watches config and manages trees
- **Tree** (`pkg/tree/`) - Service lifecycle management (start/stop/restart)
- **Arborist** (`pkg/arborist/`) - Client library for interacting with Pine
