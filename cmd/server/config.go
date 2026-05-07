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

package main

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port        string
	DBPath      string
	DatabaseURL string
	MaxUploadMB int64
	LogLevel    string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string
	S3UseSSL    bool

	CORSOrigins []string
}

func loadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DBPath:      getEnv("DB_PATH", "./uploader.db"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		MaxUploadMB: getEnvInt64("MAX_UPLOAD_MB", 50),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		S3Endpoint:  getEnv("S3_ENDPOINT", "minio:9000"),
		S3AccessKey: getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey: getEnv("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:    getEnv("S3_BUCKET", "uploads"),
		S3Region:    getEnv("S3_REGION", ""),
		S3UseSSL:    getEnvBool("S3_USE_SSL", false),

		CORSOrigins: getEnvStringSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getEnvStringSlice(key string, def []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return def
}
