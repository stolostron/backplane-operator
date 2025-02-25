// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"time"
)

const (
	/*
		ErrorRefreshInterval is used for handling critical errors that require immediate attention.
	*/
	ErrorRefreshInterval = 30 * time.Second

	/*
		FastRefreshInterval is useful for rapidly changing environments where frequent updates are needed.
	*/
	FastRefreshInterval = 1 * time.Minute

	/*
		ShortRefreshInterval is ideal for frequently changing or moderately critical state requiring timely updates.
	*/
	ShortRefreshInterval = 5 * time.Minute

	/*
		WarningRefreshInterval is suitable for addressing warnings or non-critical issues that should still be addressed
		relatively promptly.
	*/
	WarningRefreshInterval = 1 * time.Minute

	/*
		DefaultRefreshInterval serves as a fallback for any other conditions not explicitly covered by the above
		intervals.
	*/
	DefaultRefreshInterval = 20 * time.Minute
)
