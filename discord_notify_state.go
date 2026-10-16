package main

import "time"

// workerNotifyState tracks per-user, per-worker notification state.
type workerNotifyState struct {
	Online bool
	Since  time.Time

	SeenOnline  bool
	SeenOffline bool

	// OfflineEligible is set when an active worker becomes offline or stops
	// submitting shares. Workers already inactive at startup/reset stay quiet.
	OfflineEligible bool

	OfflineNotified bool

	RecoveryEligible bool
	RecoveryNotified bool
}
