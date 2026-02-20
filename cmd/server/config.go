package main

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DBPath      string
	MaxUploadMB int64
	LogLevel    string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string
	S3UseSSL    bool
}

func loadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DBPath:      getEnv("DB_PATH", "./uploader.db"),
		MaxUploadMB: getEnvInt64("MAX_UPLOAD_MB", 50),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		S3Endpoint:  getEnv("S3_ENDPOINT", "minio:9000"),
		S3AccessKey: getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey: getEnv("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:    getEnv("S3_BUCKET", "uploads"),
		S3Region:    getEnv("S3_REGION", ""),
		S3UseSSL:    getEnvBool("S3_USE_SSL", false),
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
