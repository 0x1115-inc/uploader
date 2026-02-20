package logx

import (
	"log"
	"strings"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

var currentLevel = Info

func SetLevelFromString(v string) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		currentLevel = Debug
	case "info", "":
		currentLevel = Info
	case "warn", "warning":
		currentLevel = Warn
	case "error":
		currentLevel = Error
	default:
		currentLevel = Info
		log.Printf("[WARN] invalid LOG_LEVEL=%q, using info", v)
	}
}

func Debugf(format string, args ...any) {
	if currentLevel <= Debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func Infof(format string, args ...any) {
	if currentLevel <= Info {
		log.Printf("[INFO] "+format, args...)
	}
}

func Warnf(format string, args ...any) {
	if currentLevel <= Warn {
		log.Printf("[WARN] "+format, args...)
	}
}

func Errorf(format string, args ...any) {
	if currentLevel <= Error {
		log.Printf("[ERROR] "+format, args...)
	}
}
