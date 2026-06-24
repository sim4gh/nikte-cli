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

// ValidateProfile reports whether profile is a usable WhatsApp profile (1-4).
func ValidateProfile(profile int) error {
	if profile < 1 || profile > 4 {
		return fmt.Errorf("profile must be between 1 and 4, got %d", profile)
	}
	return nil
}

// dbFileName maps a profile number to its SQLite filename. Profile 1 keeps the
// historical "whatsapp.db" name for backward compatibility; 2-4 are suffixed.
func dbFileName(profile int) (string, error) {
	if err := ValidateProfile(profile); err != nil {
		return "", err
	}
	if profile == 1 {
		return "whatsapp.db", nil
	}
	return fmt.Sprintf("whatsapp-%d.db", profile), nil
}

// sqliteDSN builds the SQLite DSN with the pragmas whatsmeow needs (WAL +
// busy_timeout so the multi-connection pool's background writers and the
// foreground readers coexist instead of returning SQLITE_BUSY).
func sqliteDSN(dbPath string) string {
	return "file:" + dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
}

// configDir returns the per-OS nikte config directory, creating it if needed.
func configDir() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "Application Support", "nikte")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		dir = filepath.Join(appData, "nikte")
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		dir = filepath.Join(configHome, "nikte")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// GetDBPath returns the platform-specific WhatsApp database path for the given profile.
func GetDBPath(profile int) (string, error) {
	name, err := dbFileName(profile)
	if err != nil {
		return "", err
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// NewClient creates a new WhatsApp client backed by SQLite for the given profile.
// If verbose is true, logs are written to stderr for debugging.
func NewClient(profile int, verbose bool) (*whatsmeow.Client, error) {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}

	var log waLog.Logger
	if verbose {
		log = waLog.Stdout("WhatsApp", "INFO", true)
	} else {
		log = noopLogger{}
	}

	container, err := sqlstore.New(context.Background(), "sqlite", sqliteDSN(dbPath), log)
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

// IsLinked checks if a WhatsApp session database exists with data for the given profile.
func IsLinked(profile int) bool {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return false
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// DeleteDB removes the WhatsApp session database and related files for the given profile.
func DeleteDB(profile int) error {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return err
	}
	// Remove WAL and SHM files (SQLite journal)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	return os.Remove(dbPath)
}

// ProfileStatus reports a profile's link state and device JID by reading only
// the local SQLite store — it never connects to WhatsApp. It opens the store
// only when the profile is already linked and always closes the DB handle.
func ProfileStatus(profile int) (linked bool, jid string, err error) {
	if err := ValidateProfile(profile); err != nil {
		return false, "", err
	}
	if !IsLinked(profile) {
		return false, "", nil
	}
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return false, "", err
	}
	container, err := sqlstore.New(context.Background(), "sqlite", sqliteDSN(dbPath), noopLogger{})
	if err != nil {
		return false, "", err
	}
	defer container.Close()

	device, err := container.GetFirstDevice(context.Background())
	if err != nil || device.ID == nil {
		return false, "", nil
	}
	return true, device.ID.String(), nil
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
