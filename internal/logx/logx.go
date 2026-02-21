/**
uploader - A simple file upload server with S3-compatible storage support
Copyright (C) 2026 0x1115 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
**/

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
