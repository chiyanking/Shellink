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
	dataHome := getenv("SHELLINK_HOME")
	if dataHome == "" {
		if goos == "windows" {
			base := getenv("APPDATA")
			if base == "" {
				base, err = os.UserConfigDir()
				if err != nil {
					return Config{}, err
				}
			}
			dataHome = filepath.Join(base, "Shellink")
		} else {
			dataHome = filepath.Join(userHome, ".Shellink")
		}
	}
	dataHome, err = filepath.Abs(dataHome)
	if err != nil {
		return Config{}, err
	}
	if err = os.MkdirAll(dataHome, 0700); err != nil {
		return Config{}, fmt.Errorf("create data directory: %w", err)
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
