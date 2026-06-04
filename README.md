# nikte CLI (Go)

A fast, single-binary CLI tool for ephemeral content management. This is a Go port of the original Node.js CLI with significantly faster startup time.

## Features

- **Fast startup**: ~20ms vs ~300ms (15x faster than Node.js version)
- **Single binary**: No runtime dependencies
- **Cross-platform**: macOS, Linux, Windows
- **OAuth 2.0 Device Flow**: Secure authentication
- **Automatic token refresh**: Seamless authentication management
- **Multiple content types**: Text, files, screenshots (macOS)
- **Screen recording**: Record screen to GIF, MP4, or MOV (macOS, requires ffmpeg for GIF/MP4)
- **TTL-based expiration**: Automatic content deletion
- **Pro features**: Sharing capabilities (Pro subscription)

## Installation

### Homebrew (recommended)

```bash
brew tap sim4gh/nikte
brew install nikte
```

### Download binary

Download the latest release from the [releases page](https://github.com/sim4gh/nikte-cli/releases) and add to your PATH.

### Build from source

```bash
git clone https://github.com/sim4gh/nikte-cli.git
cd nikte-cli
make build
make install
```

## Quick Start

```bash
# Login
nk auth login

# Add text content
nk a "Hello, World!"

# Add from clipboard
nk a    # or: nk c

# Take screenshot (macOS)
nk a sc  # or: nk sc

# Record screen to GIF (macOS, requires ffmpeg)
nk rec
nk rec -s              # Select region
nk rec --format mp4    # MP4 format

# Add file
nk a document.pdf

# List items
nk ls

# Get item
nk g <id>

# Delete item
nk d <id>
```

## Commands

### Authentication

```bash
nk auth login     # Login using device flow
nk auth logout    # Clear credentials
nk auth whoami    # Show current user
```

### Content Management

```bash
# Add content
nk a [input]              # Add from clipboard/file/text
nk a sc                   # Screenshot (macOS)
nk a document.pdf         # File upload
nk a "Hello"              # Text content
nk a --permanent          # No expiration
nk a --ttl 7d             # Custom TTL

# Get content
nk g <id>                 # Download/display item
nk g <id> --url           # Get URL only
nk g <id> --copy          # Copy URL to clipboard
nk g <id> -o ~/Downloads  # Save to directory

# List content
nk ls                     # List all items
nk ls --type text         # Filter by type
nk ls --search "query"    # Search items
nk ls --sort size         # Sort by size
nk ls --raw               # JSON output

# Delete content
nk d <id>                 # Delete with confirmation
nk d <id> --force         # Delete without confirmation

# Extend TTL
nk extend <id> --ttl 7d   # Extend to 7 days
nk extend <id> --permanent # Make permanent
```

### Screen Recording (macOS)

```bash
nk rec                        # Record fullscreen 10s → GIF
nk rec -s                     # Select region → record → GIF
nk rec -d 30                  # Record for 30 seconds (max 60)
nk rec --format mp4           # Record → MP4
nk rec --format mov           # Record → MOV (no ffmpeg needed)
nk rec --fps 15               # Custom frame rate (GIF only)
nk rec --width 1280           # Scale output width (0 = original)
nk rec -s --format mp4 -d 20  # Select region, 20s, MP4
```

Requires `ffmpeg` for GIF and MP4 formats (`brew install ffmpeg`). MOV format uses native `screencapture` only.

### Sharing (Pro)

```bash
nk sh <id>                # Create public share
nk sh <id> --password pw  # Password-protected share
nk sh <id> --expires 7d   # Custom expiration
nk p <id>                 # Quick public share
```

### Configuration

```bash
nk config                 # Show all config
nk config get <key>       # Get specific value
nk config set <key> <val> # Set value
nk config path            # Show config file path
nk config reset           # Clear all config
```

### Other

```bash
nk health                 # Check API health
nk --version              # Show version
nk --help                 # Show help
```

## Shortcuts

| Shortcut | Full Command |
|----------|--------------|
| `nk c` | `nk a` (clipboard) |
| `nk sc` | `nk a sc` (screenshot) |
| `nk p <id>` | `nk sh <id> --public` |

## Configuration

Configuration is stored in:
- macOS: `~/Library/Application Support/nikte/config.json`
- Linux: `~/.config/nikte/config.json`
- Windows: `%APPDATA%/nikte/config.json`

## TTL Format

- `30s` - 30 seconds
- `60m` - 60 minutes
- `24h` - 24 hours (default)
- `7d` - 7 days

Maximum TTL: 365 days (1 year)

## Development

```bash
make build              # Build for current platform
make build-all          # Build for all platforms
make test               # Run unit tests
make test-integration   # Run integration tests (requires auth)
make test-all           # Run all tests
make fmt                # Format code
make dev                # Build with race detector
make lint               # Run linter
```

### Integration Tests

Integration tests exercise the real API and live in `test/integration/`. They are guarded by the `//go:build integration` build tag so `make test` won't run them.

```bash
# Run all integration tests (requires prior `nk auth login`)
make test-integration

# Run just the health check (no auth needed)
go test -v -tags=integration -run TestHealthEndpoint ./test/integration/
```

In CI, the `nikte_REFRESH_TOKEN` GitHub secret provides authentication.

## Architecture

```
nikte-cli/
├── cmd/nk/main.go              # Entry point
├── internal/
│   ├── api/client.go            # HTTP client with auto-refresh
│   ├── auth/                    # OAuth, JWT, Cognito
│   │   ├── cognito.go           # Token refresh
│   │   ├── device_flow.go       # OAuth 2.0 Device Flow
│   │   └── token.go             # JWT handling
│   ├── cli/                     # Command implementations (Cobra)
│   ├── config/                  # Configuration management
│   ├── platform/                # Platform-specific code (build tags)
│   ├── upload/                  # S3 multipart upload
│   └── util/                    # TTL parsing, formatting
├── test/integration/            # Integration tests
│   ├── helpers_test.go          # TestMain, auth setup, helpers
│   └── integration_test.go      # API test cases
├── .github/workflows/
│   ├── release.yml              # GoReleaser on tag push
│   └── integration.yml          # Integration tests CI
├── go.mod
├── Makefile
└── README.md
```

## License

ISC
