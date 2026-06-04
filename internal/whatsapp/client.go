package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	signalLogger "go.mau.fi/libsignal/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite"
)

// noopLogger implements waLog.Logger with no output
type noopLogger struct{}

func (noopLogger) Warnf(string, ...interface{})  {}
func (noopLogger) Errorf(string, ...interface{}) {}
func (noopLogger) Infof(string, ...interface{})  {}
func (noopLogger) Debugf(string, ...interface{}) {}
func (noopLogger) Sub(string) waLog.Logger       { return noopLogger{} }

// silentSignalLogger silences libsignal's own global logger
type silentSignalLogger struct{}

func (silentSignalLogger) Debug(string, string)   {}
func (silentSignalLogger) Info(string, string)    {}
func (silentSignalLogger) Warning(string, string) {}
func (silentSignalLogger) Error(string, string)   {}
func (silentSignalLogger) Configure(string)       {}

func init() {
	// Silence libsignal's global logger (separate from whatsmeow's logger)
	var sl signalLogger.Loggable = &silentSignalLogger{}
	signalLogger.Setup(&sl)
}

// GetDBPath returns the platform-specific WhatsApp database path
func GetDBPath() (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, "Library", "Application Support", "nikte")

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		configDir = filepath.Join(appData, "nikte")

	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		configDir = filepath.Join(configHome, "nikte")
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "whatsapp.db"), nil
}

// NewClient creates a new WhatsApp client backed by SQLite.
// If verbose is true, logs are written to stderr for debugging.
func NewClient(verbose bool) (*whatsmeow.Client, error) {
	dbPath, err := GetDBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}

	var log waLog.Logger
	if verbose {
		log = waLog.Stdout("WhatsApp", "INFO", true)
	} else {
		log = noopLogger{}
	}

	// WAL mode + a busy_timeout are required: whatsmeow opens a multi-connection
	// pool and writes from background goroutines right after Connect(), while
	// foreground calls like SendMessage read concurrently (e.g. "fetch LID
	// mappings"). Without busy_timeout, SQLite returns SQLITE_BUSY immediately
	// instead of waiting for the lock; WAL lets readers and writers coexist.
	dsn := "file:" + dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	container, err := sqlstore.New(context.Background(), "sqlite", dsn, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, log)
	return client, nil
}

// IsLinked checks if a WhatsApp session database exists with data
func IsLinked() bool {
	dbPath, err := GetDBPath()
	if err != nil {
		return false
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// DeleteDB removes the WhatsApp session database and related files
func DeleteDB() error {
	dbPath, err := GetDBPath()
	if err != nil {
		return err
	}
	// Remove WAL and SHM files (SQLite journal)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	return os.Remove(dbPath)
}

// FormatNumber cleans a phone number and returns a WhatsApp JID
func FormatNumber(number string) types.JID {
	clean := ""
	for _, c := range number {
		if c >= '0' && c <= '9' {
			clean += string(c)
		}
	}
	return types.JID{
		User:   clean,
		Server: types.DefaultUserServer,
	}
}
