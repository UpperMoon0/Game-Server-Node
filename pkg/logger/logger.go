package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a new logger instance
func New(logLevel string, logFormat string, logFilePath string) (*zap.Logger, error) {
	// Parse log level
	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "fatal":
		level = zapcore.FatalLevel
	default:
		level = zapcore.InfoLevel
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey: "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder
	var encoder zapcore.Encoder
	if logFormat == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create cores
	var cores []zapcore.Core

	// Console output
	consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	cores = append(cores, consoleCore)

	// File output if path specified
	if logFilePath != "" {
		logDir := filepath.Dir(logFilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, err
		}

		currentTime := time.Now().Format("2006-01-02")
		filePath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", filepath.Base(logFilePath), currentTime))
		
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}

		fileCore := zapcore.NewCore(encoder, zapcore.AddSync(file), level)
		cores = append(cores, fileCore)
	}

	core := zapcore.NewTee(cores...)
	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}
