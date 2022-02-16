package main

import (
	"fmt"

	"github.com/opensourceways/community-robot-lib/config"
)

type configuration struct {
	Config botConfig `json:"config,omitempty"`
}

func (c *configuration) Validate() error {
	if c == nil {
		return nil
	}

	return c.Config.validate()
}

func (c *configuration) SetDefault() {
}

type botConfig struct {
	config.RepoFilter

	// MergeRemindIntervals Indicates the time interval for adding merge reminders.
	// When the current time minus the PR creation time is greater than this configuration,
	// a merge reminder needs to be added. In addition, the same is true when the last merge
	// reminder and the current time interval exceeds this configuration item. Unit day.
	MergeRemindIntervals int `json:"merge_remind_intervals" required:"true"`

	// MaximumOpenTime indicates the maximum time a PR can remain open. Unit day.
	MaximumOpenTime int `json:"maximum_open_time" required:"true"`

	// ConcurrentSize is the concurrent size for doing task
	ConcurrentSize int `json:"concurrent_size" required:"true"`
}

func (c *botConfig) validate() error {
	if c.ConcurrentSize <= 0 {
		return fmt.Errorf("concurrent_size must be bigger than 0")
	}

	if c.MergeRemindIntervals <= 0 {
		return fmt.Errorf("merge_remind_intervals must be bigger than 0")
	}

	if c.MaximumOpenTime <= 0 {
		return fmt.Errorf("maximum_open_time must be bigger than 0")
	}

	return c.RepoFilter.Validate()
}
