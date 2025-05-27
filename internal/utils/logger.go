package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	// "github.com/mattn/go-colorable"
	"path/filepath"
	"io"
)

var (
    Logger zerolog.Logger
)

func init() {
    // Log to both file and console
    logFile, err := os.OpenFile("run.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        panic(err)
    }

    // Console writer with color
    consoleWriter := zerolog.ConsoleWriter{Out: goColorableStdout(), TimeFormat: "15:04:05",
	FormatCaller: func(i interface{}) string {
		return filepath.Base(i.(string)) // Show only the filename, not full path
		},
	FormatFieldName: func(i interface{}) string {
		return fmt.Sprintf("|%s|", i) // Add vertical bars around field names
	},
	}

    consoleWriter.FormatLevel = func(i interface{}) string {
		level := strings.ToUpper(i.(string))
        // Colored levels
        switch level {
        case "DEBUG":
            return "\033[36m[" + level + "]\033[0m"
        case "INFO":
            return "\033[32m[" + level + "]\033[0m"
        case "WARN":
            return "\033[33m[" + level + "]\033[0m"
        case "ERROR":
            return "\033[31m[" + level + "]\033[0m"
        default:
            return level
        }
    }

	// todo when in production, use env variable to switch off console log
	// if os.Getenv("GO_ENV") == "production" {
	// 	consoleWriter.Out = io.Discard // Disable console output in production
	// } 

    multi := io.MultiWriter(consoleWriter, logFile)
    zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
    Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()

    // Also replace global log, so log.Info().Msg() etc works everywhere
    log.Logger = Logger
}

// Windows/ANSI-safe colorable output
func goColorableStdout() *os.File {
    return os.Stdout
}


