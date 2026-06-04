# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

nikte CLI (Go) is a fast, single-binary command-line tool for ephemeral content management. This is a Go port of the original Node.js CLI (`oio-cli/`) with significantly faster startup time (~25x).

Features:
- OAuth 2.0 Device Flow authentication
- Unified content management (text, files, screenshots)
- Screen recording to GIF, MP4, or MOV (macOS, requires ffmpeg for GIF/MP4)
- TTL-based auto-deletion (default: 24h)
- Sharing capabilities (Pro subscription)
- Cross-platform support (macOS, Linux, Windows)

## Development Commands

### Build
```bash
make build          # Build for current platform
make build-all      # Build for all platforms (macOS, Linux, Windows)
go build -o nk ./cmd/nk  # Direct Go build
```

### Test Locally
```bash
# Link for local testing
ln -sf "$(pwd)/oio" ~/bin/nikte-cli

# Test commands
./nk --version
./nk health
./nk auth login
./nk a "Hello"
./nk ls
```

### Install
```bash
make install        # Install to /usr/local/bin (requires sudo)
```

### Clean
```bash
make clean          # Remove build artifacts
```

## Architecture

### Directory Structure
```
nikte-cli/
├── cmd/nk/main.go              # Entry point
├── internal/
│   ├── api/client.go            # HTTP client with auto-refresh
│   ├── auth/
│   │   ├── cognito.go           # Token refresh via Cognito
│   │   ├── device_flow.go       # OAuth 2.0 Device Flow
│   │   └── token.go             # JWT decode, expiration check
│   ├── cli/                     # Command implementations (Cobra)
│   │   ├── root.go              # Root command setup
│   │   ├── auth.go              # login, logout, whoami
│   │   ├── add.go               # add command (text, file, screenshot)
│   │   ├── get.go               # get/download command
│   │   ├── list.go              # list command
│   │   ├── delete.go            # delete command
│   │   ├── extend.go            # extend TTL command
│   │   ├── share.go             # share command (Pro)
│   │   ├── config.go            # config management
│   │   ├── health.go            # health check
│   │   ├── rec.go               # screen recording (GIF/MP4/MOV)
│   │   └── shortcuts.go         # c, sc, p aliases
│   ├── config/
│   │   ├── config.go            # JSON config management
│   │   └── paths.go             # Platform-specific paths
│   ├── platform/                # Platform-specific code
│   │   ├── clipboard.go         # Clipboard detection
│   │   ├── clipboard_darwin.go  # macOS clipboard (pngpaste)
│   │   ├── clipboard_other.go   # Stub for other platforms
│   │   ├── screenshot_darwin.go  # macOS screenshot (screencapture)
│   │   ├── screenshot_other.go  # Stub for other platforms
│   │   ├── recording_darwin.go  # macOS screen recording + conversion
│   │   └── recording_other.go   # Stub for other platforms
│   ├── upload/multipart.go      # S3 multipart upload
│   └── util/
│       ├── format.go            # Byte formatting, progress bars
│       └── ttl.go               # TTL parsing and formatting
├── test/integration/            # Integration tests (build tag: integration)
│   ├── helpers_test.go          # TestMain, auth setup, test helpers
│   └── integration_test.go      # API integration test cases
├── .github/workflows/
│   ├── release.yml              # GoReleaser on tag push
│   └── integration.yml          # Integration tests CI
├── go.mod
├── Makefile
└── README.md
```

### Key Libraries
| Library | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/fatih/color` | Colored output |
| `github.com/briandowns/spinner` | Loading spinners |
| `github.com/olekukonko/tablewriter` | Table rendering |
| `github.com/atotto/clipboard` | Cross-platform clipboard |
| `github.com/pkg/browser` | Open URLs in browser |

### Hardcoded Values
```go
// internal/api/client.go
const DefaultBaseURL = "https://auth.nikte.co"

// internal/auth/cognito.go
const CognitoDomain = "nikte-fcf57b8c.auth.us-west-2.amazoncognito.com"
const ClientID = "2385ict6amoluilmqns4jf0n73"
```

### Configuration Storage
Platform-specific paths defined in `internal/config/paths.go`:
- macOS: `~/Library/Application Support/nikte/config.json`
- Linux: `~/.config/nikte/config.json`
- Windows: `%APPDATA%/nikte/config.json`

## Command Structure

```
oio
├── auth
│   ├── login              # OAuth device flow
│   ├── logout             # Clear credentials
│   └── whoami             # Show current user
├── a [input]  (alias: add)
│   ├── nk a              # From clipboard
│   ├── nk a sc           # Screenshot (macOS)
│   ├── nk a <path>       # From file
│   └── nk a "text"       # Text content
├── g <id>  (alias: get)   # Get/download item
├── ls  (alias: list)      # List all items
├── d <id>  (alias: delete)# Delete item
├── extend <id>            # Extend TTL
├── sh <id>  (alias: share)# Share item (Pro)
├── rec                    # Screen recording (GIF/MP4/MOV, macOS)
│   ├── nk rec            # Fullscreen 10s → GIF
│   ├── nk rec -s         # Select region → GIF
│   ├── nk rec -d 30      # 30 seconds
│   ├── nk rec --format mp4  # MP4 output
│   └── nk rec --format mov  # MOV (no ffmpeg)
├── wa                     # WhatsApp messaging (whatsmeow, local SQLite session)
│   ├── nk wa link        # Link account (scan QR)
│   ├── nk wa send <num> [message|file|sc] [caption]  # See below
│   ├── nk wa ls [--all]  # Unread (or all) conversations
│   ├── nk wa status      # Link status
│   └── nk wa unlink      # Clear session
├── link <url>             # URL shortener → share.nikte.co/<code>
│   ├── nk link <url>     # Shorten (--ttl 7d, --permanent; copies to clipboard)
│   ├── nk link ls        # List your short links
│   └── nk link d <code>  # Delete a short link
├── config                 # Configuration management
├── health                 # Health check
├── c                      # Quick clipboard (alias for "nk a")
├── sc                     # Quick screenshot (alias for "nk a sc")
└── p <id>                 # Quick public share
```

### WhatsApp (`wa`)

WhatsApp is handled locally via [whatsmeow](https://github.com/tulir/whatsmeow),
not the nikte backend. The session lives in a SQLite DB next to `config.json`
(`internal/whatsapp/client.go`, `GetDBPath()`), opened in **WAL mode with a
`busy_timeout`** — required because whatsmeow writes from background goroutines
while foreground calls (e.g. `SendMessage` → "fetch LID mappings") read
concurrently; without it SQLite returns `SQLITE_BUSY` immediately.

`nk wa send <number> [arg] [caption...]` auto-detects the second argument
(`runWaSend` / `buildWaSendMessage` in `internal/cli/wa.go`):

| Second arg | Behavior |
|------------|----------|
| omitted | Send clipboard content — image if the clipboard holds one, else text |
| `sc` | Capture a screenshot (region select, or full screen with `-f`) and send it |
| existing file path | Send the file; MIME type picks `ImageMessage`/`VideoMessage`/`AudioMessage`/`DocumentMessage` |
| anything else | Send as a plain text message |

Paths may be absolute, relative (`./x.png`, `../x.png`), or use `~` even when
quoted (`expandTilde` handles the quoted case; the shell handles unquoted).
Extra words after a file/`sc` become the caption.

### URL shortener (`link`)

`internal/cli/link.go` talks to the backend's `/links` endpoints:
- `nk link <url>` → `POST /links` (default 48h TTL; `--ttl`/`--permanent`).
  `https://` is prepended when the scheme is missing. Prints and clipboard-copies
  the `share.nikte.co/<code>` short URL.
- `nk link ls` → `GET /links` (tablewriter of the user's links).
- `nk link d <code>` → `DELETE /links/{code}` (confirm unless `--force`).

The public redirect (`share.nikte.co/<code>` → 302) is served by the
backend's `access-share-handler`, not the CLI.

## Adding New Commands

1. Create command file in `internal/cli/`:
```go
// internal/cli/example.go
package cli

import (
    "fmt"
    "github.com/sim4gh/nikte-cli/internal/api"
    "github.com/spf13/cobra"
)

func addExampleCommand() {
    exampleCmd := &cobra.Command{
        Use:   "example <arg>",
        Short: "Example command description",
        Args:  cobra.ExactArgs(1),
        RunE:  runExample,
    }

    exampleCmd.Flags().BoolVar(&exampleFlag, "flag", false, "Flag description")
    rootCmd.AddCommand(exampleCmd)
}

func runExample(cmd *cobra.Command, args []string) error {
    resp, err := api.Post("/endpoint", map[string]interface{}{
        "data": args[0],
    })
    if err != nil {
        return err
    }

    if resp.StatusCode != 200 {
        return fmt.Errorf("failed: %s", resp.GetString("message"))
    }

    fmt.Printf("Result: %s\n", resp.GetString("result"))
    return nil
}
```

2. Register in `internal/cli/root.go`:
```go
func init() {
    // ... existing commands
    addExampleCommand()
}
```

## API Request Pattern

All API calls use the `api` package which handles:
- Automatic token refresh when expired (60-second buffer)
- Authorization header injection
- JSON marshaling/unmarshaling
- Error handling

```go
// Authenticated requests
resp, err := api.Get("/shorts")
resp, err := api.Post("/shorts", body)
resp, err := api.Patch("/shorts/"+id, body)
resp, err := api.Delete("/shorts/"+id)

// Unauthenticated requests
resp, err := api.GetNoAuth("/health")

// Response handling
if resp.StatusCode == 200 {
    var result MyStruct
    resp.Unmarshal(&result)
}
```

## Token Management

- ID tokens are used for API authorization (not access tokens)
- Tokens are automatically refreshed in `api.Request()` before each call
- Refresh happens when token expires within 60 seconds
- JWT decoding in `internal/auth/token.go`
- Cognito refresh in `internal/auth/cognito.go`

## Platform-Specific Code

Use build tags for platform-specific implementations:

```go
//go:build darwin
// darwin-only code

//go:build !darwin
// non-darwin fallback
```

Screenshot, recording, and clipboard image features are macOS-only:
- `screencapture` command for screenshots and screen recording
- `pngpaste` (brew install pngpaste) for clipboard images
- `ffmpeg` (brew install ffmpeg) for GIF/MP4 conversion (MOV works without it)

## Error Handling

Commands should return errors rather than calling `os.Exit()`:
```go
func runCommand(cmd *cobra.Command, args []string) error {
    if err := doSomething(); err != nil {
        return fmt.Errorf("failed to do something: %w", err)
    }
    return nil
}
```

The root command handles displaying errors and exit codes.

## Testing

**Existing tests must always pass. Never delete, skip, or modify existing tests to make code changes pass. If a test fails, fix the implementation, not the test. CI runs on every push/PR to `main`.**

### Integration Tests

Integration tests exercise the real API and require authentication. They live in `test/integration/` and are guarded by the `//go:build integration` build tag.

```bash
make test-integration           # Run integration tests (requires auth)
make test-all                   # Run unit + integration tests

# Run a single test
go test -v -tags=integration -run TestHealthEndpoint -timeout=30s ./test/integration/

# Health endpoint only (no auth needed)
go test -v -tags=integration -run TestHealthEndpoint ./test/integration/
```

**Auth setup:** Locally, tests use the refresh token from `~/Library/Application Support/nikte/config.json`. In CI, the `nikte_REFRESH_TOKEN` GitHub secret is used.

**Test coverage:**
- `TestHealthEndpoint` — no-auth connectivity check
- `TestShortsCRUDLifecycle` — Create, Get, List, Extend, MakePermanent, Delete, Verify404
- `TestShortsCreateValidation` — empty content returns 400
- `TestShortsGetNotFound` — nonexistent ID returns 404
- `TestShortsDeleteIdempotent` — double delete: first succeeds, second 404
- `TestScreenshotCRUD` — create (1x1 PNG), get (verify downloadUrl), list, delete
- `TestConcurrentOperations` — 3 parallel creates, verify all succeed

### Manual Testing

```bash
./nk health                    # No auth required
./nk auth login                # Complete device flow
./nk a "test content"          # Add text
./nk ls                        # List items
./nk g <id>                    # Get item
./nk d <id> --force            # Delete item
./nk auth logout               # Clear credentials
```

## Performance

| Metric | Go | Node.js | Improvement |
|--------|-----|---------|-------------|
| Startup | ~13ms | ~327ms | 25x faster |
| Binary | 12MB | N/A | Single file |

## Release Process

1. Update version in `internal/cli/root.go`:
   ```go
   var Version = "1.0.1"
   ```

2. Build release binaries:
   ```bash
   VERSION=1.0.1 make build-all
   ```

3. Binaries are created in `build/`:
   - `oio-darwin-arm64`
   - `oio-darwin-amd64`
   - `oio-linux-amd64`
   - `oio-linux-arm64`
   - `oio-windows-amd64.exe`

4. Create GitHub release with binaries

5. Update Homebrew formula (if distributing via Homebrew)
