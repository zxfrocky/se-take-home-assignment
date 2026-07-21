package order

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type EventLogger struct {
	mu sync.Mutex
	w  io.Writer
}

func NewEventLogger(w io.Writer) *EventLogger {
	return &EventLogger{w: w}
}

func (l *EventLogger) Header() {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, "McDonald's Order Management System - Simulation Results")
	fmt.Fprintln(l.w)
}

func (l *EventLogger) Event(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func (l *EventLogger) BlankLine() {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w)
}

func (l *EventLogger) Line(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, format+"\n", args...)
}
