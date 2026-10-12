// Package service is what every benchmark server shares: the work an Echo
// call does, the -addr flag, and the signal that stops the server.
//
// The work is kitex-benchmark's: a call's Action is "echo", which sends Msg
// straight back, or "sleep", whose Msg starts with "<milliseconds>," and which
// holds the call that long before echoing, to stand in for a handler that
// waits on something.
package service

import (
	"flag"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// ActionEcho sends Msg back as it came.
	ActionEcho = "echo"
	// ActionSleep sleeps for the milliseconds Msg starts with, then echoes.
	ActionSleep = "sleep"
)

// Echo does the work of one call and returns the reply's Action and Msg.
func Echo(action, msg string) (string, string) {
	if action == ActionSleep {
		if i := strings.IndexByte(msg, ','); i > 0 {
			if ms, err := strconv.Atoi(msg[:i]); err == nil && ms > 0 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
	}
	return action, msg
}

// Flags parses the server's flags and returns the address to listen on.
func Flags(defaultAddr string) string {
	addr := flag.String("addr", defaultAddr, "listen address")
	flag.Parse()
	return *addr
}

// WaitSignal blocks until the process is asked to stop: SIGINT or SIGTERM,
// which is how script/benchmark.sh ends a server after its turn.
func WaitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
