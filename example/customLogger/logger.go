package main

import (
	"io"
	"log"
	"os"
)

// Logger is a logger that implements statute.Logger to be able to be used in mixed.WithLogger
type Logger struct {
	debugLog *log.Logger
	errorLog *log.Logger
}

// NewLogger creates the log file and configures logger to write to both file & Standard streams(Stdout, Stderr)
func NewLogger(filename string) (*Logger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	errorMultiWriter := io.MultiWriter(file, os.Stderr)
	debugMultiWriter := io.MultiWriter(file, os.Stdout)

	return &Logger{
		errorLog: log.New(errorMultiWriter, "ERROR: ", log.Ldate|log.Ltime),
		debugLog: log.New(debugMultiWriter, "DEBUG: ", log.Ldate|log.Ltime),
	}, nil
}

func (l *Logger) Debug(v ...interface{}) {
	l.debugLog.Println(v...)
}

func (l *Logger) Error(v ...interface{}) {
	l.errorLog.Println(v...)
}
