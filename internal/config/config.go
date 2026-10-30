package config

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	DataHome                 string
	DBPath                   string
	SocketPath               string
	PipeName                 string
	LogPath                  string
	PIDPath                  string
	Host                     string
	Port                     int
	HTTPEnabled              bool
	Token                    string
	RequireTokenForSensitive *bool
	MasterKey                [32]byte
	MaxFrameBytes            int
	SocketMaxQueueBytes      int
	SilenceThresholdMS       int
	ExecDefaultTimeoutMS     int
	TransferMaxBytes         int
	TransferTimeoutMS        int
	EditTimeoutMS            int
	SSHReadyTimeoutMS        int
}

func Load() (Config, error) { return LoadFrom(os.Getenv, runtime.GOOS) }

func LoadFrom(getenv func(string) string, goos string) (Config, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	dataHome, err := resolveDataHome(getenv, goos, userHome)
	if err != nil {
		return Config{}, err
	}
	dataHome, err = filepath.Abs(dataHome)
	if err != nil {
		return Config{}, err
	}
	if err = os.MkdirAll(dataHome, 0700); err != nil {
		return Config{}, fmt.Errorf("create data directory %s: %w (point SHELLINK_HOME at a writable folder)", dataHome, err)
	}

	dbPath := getenv("SHELLINK_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dataHome, "shellink.db")
	} else if dbPath, err = filepath.Abs(dbPath); err != nil {
		return Config{}, err
	}
	logPath := getenv("SHELLINK_LOG")
	if logPath == "" {
		logPath = filepath.Join(dataHome, "shellink.log")
	} else if logPath, err = filepath.Abs(logPath); err != nil {
		return Config{}, err
	}

	pipeName := ""
	socketPath := getenv("SHELLINK_SOCKET")
	if goos == "windows" {
		if socketPath == "" {
			current, e := user.Current()
			if e != nil || current.Username == "" {
				pipeName = `\\.\pipe\shellink`
			} else {
				pipeName = `\\.\pipe\shellink-` + strings.NewReplacer(`\`, "_", "/", "_").Replace(current.Username)
			}
		} else {
			pipeName = socketPath
		}
	} else if socketPath == "" {
		if goos == "darwin" {
			tmp := getenv("TMPDIR")
			if tmp == "" {
				tmp = os.TempDir()
			}
			socketPath = filepath.Join(tmp, "shellink-"+numericUID(), "shellink.sock")
		} else if runtimeDir := getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			socketPath = filepath.Join(runtimeDir, "shellink", "shellink.sock")
		} else {
			socketPath = filepath.Join(os.TempDir(), "shellink-"+numericUID(), "shellink.sock")
		}
	} else {
		socketPath, err = filepath.Abs(socketPath)
		if err != nil {
			return Config{}, err
		}
	}

	maxFrame, err := intEnv(getenv, "SHELLINK_MAX_FRAME_BYTES", 16<<20)
	if err != nil || maxFrame < 1024 {
		return Config{}, fmt.Errorf("SHELLINK_MAX_FRAME_BYTES is invalid")
	}
	transferMax, err := intEnv(getenv, "SHELLINK_TRANSFER_MAX_BYTES", 6_291_456)
	if err != nil || transferMax < 0 || transferMax > maxFrame {
		return Config{}, fmt.Errorf("the file transfer limit cannot exceed the socket frame limit")
	}
	port, err := intEnv(getenv, "SHELLINK_PORT", 7070)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("SHELLINK_PORT is invalid")
	}
	httpEnabled, err := boolEnv(getenv, "SHELLINK_HTTP_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	requireToken, err := optionalBoolEnv(getenv, "SHELLINK_REQUIRE_TOKEN_FOR_SENSITIVE_OPS")
	if err != nil {
		return Config{}, err
	}
	masterKey, err := loadMasterKey(dataHome, getenv("SHELLINK_MASTER_KEY"), goos)
	if err != nil {
		return Config{}, err
	}
	queueMax, err := intEnv(getenv, "SHELLINK_SOCKET_MAX_QUEUE_BYTES", 4<<20)
	if err != nil || queueMax < 1024 {
		return Config{}, fmt.Errorf("SHELLINK_SOCKET_MAX_QUEUE_BYTES is invalid")
	}
	host := getenv("SHELLINK_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	token := getenv("SHELLINK_TOKEN")
	if token == "" {
		token = "change-me"
	}
	return Config{
		DataHome: dataHome, DBPath: dbPath, SocketPath: socketPath, PipeName: pipeName,
		LogPath: logPath, PIDPath: filepath.Join(dataHome, "shellink.pid"),
		Host: host, Port: port, HTTPEnabled: httpEnabled, Token: token,
		RequireTokenForSensitive: requireToken, MasterKey: masterKey,
		MaxFrameBytes: maxFrame, SocketMaxQueueBytes: queueMax,
		SilenceThresholdMS:   intEnvDefault(getenv, "SHELLINK_SILENCE_MS", 800),
		ExecDefaultTimeoutMS: intEnvDefault(getenv, "SHELLINK_EXEC_TIMEOUT_MS", 20_000),
		TransferMaxBytes:     transferMax,
		TransferTimeoutMS:    intEnvDefault(getenv, "SHELLINK_TRANSFER_TIMEOUT_MS", 120_000),
		EditTimeoutMS:        intEnvDefault(getenv, "SHELLINK_EDIT_TIMEOUT_MS", 25_000),
		SSHReadyTimeoutMS:    intEnvDefault(getenv, "SHELLINK_SSH_READY_TIMEOUT_MS", 30_000),
	}, nil
}

// resolveDataHome picks the directory that keeps Shellink state (database,
// master key, logs, MCP settings).
//
// The per-user profile directory is only a convention. The executable may be
// copied to another drive, unpacked from a portable archive or started on a
// machine where %APPDATA% is locked down, so:
//   - an explicit SHELLINK_HOME always wins;
//   - otherwise the per-user default is used, as long as it is writable;
//   - a default that already holds a database is kept even when the writability
//     probe fails, because a transient failure must not look like lost history;
//   - otherwise the state moves next to the executable, with a warning, so the
//     app still starts instead of dying on a locked-down profile directory.
//
// The portable folder must never take precedence merely because it exists:
// that silently switches the database, so a user who unzips the binaries into
// a new folder would find an empty session list while the real history stayed
// behind in the old location. Choose it explicitly with SHELLINK_HOME instead.
func resolveDataHome(getenv func(string) string, goos, userHome string) (string, error) {
	return resolveDataHomeWith(getenv, goos, userHome, portableDataHome())
}

// writableProbe reports whether Shellink may keep its state in a directory. It
// is a variable so tests can simulate a directory that is momentarily locked.
var writableProbe = writableDirectory

// resolveDataHomeWith is resolveDataHome with the portable fallback injected so
// the precedence rules can be tested without writing next to the test binary.
func resolveDataHomeWith(getenv func(string) string, goos, userHome, portable string) (string, error) {
	if explicit := getenv("SHELLINK_HOME"); explicit != "" {
		return explicit, nil
	}
	fallback, err := defaultDataHome(getenv, goos, userHome)
	if err != nil {
		return "", err
	}
	if writableProbe(fallback) {
		return fallback, nil
	}
	// The probe can fail for reasons that pass (antivirus holding the folder for
	// a moment, a slow network profile). Never walk away from a directory that
	// already holds a database: a transient failure would start the app on an
	// empty data set and look exactly like lost history. Surfacing the write
	// error is far better than silently switching stores.
	if hasState(fallback) {
		return fallback, nil
	}
	if portable == "" || !writableDirectory(portable) {
		return "", fmt.Errorf("no writable data directory: %s cannot be created (point SHELLINK_HOME at a writable folder)", fallback)
	}
	fmt.Fprintf(os.Stderr, "Shellink: %s is not writable, keeping data in %s\n", fallback, portable)
	return portable, nil
}

// hasState reports whether a directory already keeps Shellink data, which makes
// it authoritative regardless of whether the writability probe passed.
func hasState(dir string) bool {
	for _, name := range []string{"shellink.db", "master.key"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func defaultDataHome(getenv func(string) string, goos, userHome string) (string, error) {
	if goos != "windows" {
		return filepath.Join(userHome, ".Shellink"), nil
	}
	base := getenv("APPDATA")
	if base == "" {
		resolved, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		base = resolved
	}
	return filepath.Join(base, "Shellink"), nil
}

// portableDataHome is the fallback folder beside the executable. It is only
// used when the per-user directory cannot be written, which is the case for a
// locked-down %APPDATA% or a copy running from a read-only location.
func portableDataHome() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(executable), "shellink-data")
}

func writableDirectory(path string) bool {
	return CheckDataHome(path) == nil
}

// CheckDataHome explains why a data directory cannot be used, naming the exact
// directory. A failed start otherwise blames "the data directory" in general
// and leaves the user guessing whether it was the folder, the log file or the
// database that the system refused.
func CheckDataHome(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("data directory %s cannot be created: %w", path, err)
	}
	probe, err := os.CreateTemp(path, ".shellink-probe-*")
	if err != nil {
		return fmt.Errorf("data directory %s is not writable: %w%s", path, err, lowIntegrityHint())
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// lowIntegrityHint explains the Windows cause that looks like a data directory
// problem but is not: an executable stored in a folder marked "low integrity"
// (browser downloads, extracted archives) runs with a low integrity token, and
// Windows then refuses every write into the user profile. The data directory is
// perfectly writable for any other process, so without this note the only
// visible symptom is a bare "Access is denied".
func lowIntegrityHint() string {
	temp, err := os.CreateTemp("", "shellink-write-*")
	if err != nil {
		executable, exeErr := os.Executable()
		if exeErr != nil {
			return ""
		}
		folder := filepath.Dir(executable)
		return fmt.Sprintf("\n  the process cannot write %s either, so the problem is not this folder: %s is probably marked low integrity (typical for downloaded files). Copy the folder somewhere else, or run: icacls \"%s\" /setintegritylevel (OI)(CI)Medium /T", os.TempDir(), folder, folder)
	}
	name := temp.Name()
	_ = temp.Close()
	_ = os.Remove(name)
	return ""
}

func numericUID() string {
	current, err := user.Current()
	if err == nil && current.Uid != "" {
		return current.Uid
	}
	return "user"
}

func intEnv(getenv func(string) string, key string, fallback int) (int, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func intEnvDefault(getenv func(string) string, key string, fallback int) int {
	n, err := intEnv(getenv, key, fallback)
	if err != nil {
		return fallback
	}
	return n
}

func boolEnv(getenv func(string) string, key string, fallback bool) (bool, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}
	return parseBool(raw, key)
}

func optionalBoolEnv(getenv func(string) string, key string) (*bool, error) {
	raw := getenv(key)
	if raw == "" {
		return nil, nil
	}
	v, err := parseBool(raw, key)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func parseBool(raw, key string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean (true/false)", key)
	}
}

func loadMasterKey(home, envHex, goos string) ([32]byte, error) {
	var key [32]byte
	path := filepath.Join(home, "master.key")
	var envKey *[32]byte
	if envHex != "" {
		decoded, err := hex.DecodeString(envHex)
		if err != nil || len(decoded) != len(key) {
			return key, fmt.Errorf("SHELLINK_MASTER_KEY must be a 32-byte hex string (64 hex characters)")
		}
		var parsed [32]byte
		copy(parsed[:], decoded)
		envKey = &parsed
	}
	if content, err := os.ReadFile(path); err == nil {
		if goos != "windows" {
			info, statErr := os.Stat(path)
			if statErr != nil || info.Mode().Perm() != 0600 {
				return key, fmt.Errorf("%s permissions must be 0600", path)
			}
		}
		decoded, decodeErr := hex.DecodeString(strings.TrimSpace(string(content)))
		if decodeErr != nil || len(decoded) != len(key) {
			return key, fmt.Errorf("invalid master key file: %s", path)
		}
		copy(key[:], decoded)
		if envKey != nil && subtle.ConstantTimeCompare(key[:], envKey[:]) != 1 {
			return [32]byte{}, fmt.Errorf("SHELLINK_MASTER_KEY does not match the existing master.key")
		}
		return key, nil
	} else if !os.IsNotExist(err) {
		return key, err
	}
	if envKey != nil {
		key = *envKey
	} else if _, err := rand.Read(key[:]); err != nil {
		return key, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key[:])+"\n"), 0600); err != nil {
		return [32]byte{}, err
	}
	return key, nil
}
