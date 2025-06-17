package writetarget

import (
	"time"
)

// Config defines the write target component configuration.
type Config struct {
	PollPeriod        time.Duration
	AcceptanceTimeout time.Duration
}

// DefaultConfigSet is the default configuration for the write target component.
var DefaultConfigSet = Config{
	PollPeriod:        1 * time.Second,
	AcceptanceTimeout: 10 * time.Second,
}
