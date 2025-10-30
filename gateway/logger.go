package gateway

import (
    "log"
)

// LogLevel 表示日志级别
type LogLevel int

const (
    LevelError LogLevel = iota
    LevelWarn
    LevelInfo
    LevelDebug
)

// 当前日志级别：当 config.Debug 为 true 时使用 Debug，否则使用 Info
func currentLevel() LogLevel {
    if config != nil && config.Debug {
        return LevelDebug
    }
    return LevelInfo
}

func logf(level LogLevel, format string, v ...interface{}) {
    if level > currentLevel() {
        return
    }
    switch level {
    case LevelError:
        log.Printf("[ERROR] "+format, v...)
    case LevelWarn:
        log.Printf("[WARN] "+format, v...)
    case LevelInfo:
        log.Printf("[INFO] "+format, v...)
    case LevelDebug:
        log.Printf("[DEBUG] "+format, v...)
    }
}

func Errorf(format string, v ...interface{}) { logf(LevelError, format, v...) }
func Warnf(format string, v ...interface{})  { logf(LevelWarn, format, v...) }
func Infof(format string, v ...interface{})  { logf(LevelInfo, format, v...) }
func Debugf(format string, v ...interface{}) { logf(LevelDebug, format, v...) }


