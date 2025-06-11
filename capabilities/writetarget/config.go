package writetarget

import (
	"time"
)

// Config defines the write target component configuration.
type Config struct {
	ConfirmerPollPeriod time.Duration
	ConfirmerTimeout    time.Duration
}

// DefaultConfigSet is the default configuration for the write target component.
var DefaultConfigSet = Config{
	ConfirmerPollPeriod: 1 * time.Second,
	ConfirmerTimeout:    10 * time.Second,
}
