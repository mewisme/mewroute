package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Env struct {
	RootDir      string
	Port         int
	LogLevel     string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func LoadEnv() (Env, error) {
	e := Env{
		RootDir:      getenv("ROOT_DIR", "/data"),
		LogLevel:     getenv("LOG_LEVEL", "info"),
		ReadTimeout:  durationEnv("READ_TIMEOUT", 30*time.Second),
		WriteTimeout: durationEnv("WRITE_TIMEOUT", 60*time.Second),
	}

	portStr := getenv("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return Env{}, fmt.Errorf("invalid PORT %q", portStr)
	}
	e.Port = port
	return e, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
