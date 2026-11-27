//go:build windows

package main

import "golang.org/x/sys/windows/svc/eventlog"

const startupEventSource = "EsteriumERPStartup"
const startupEventID = 1

func writeWindowsStartupDiagnostic(message string) error {
	log, err := eventlog.Open(startupEventSource)
	if err != nil {
		return err
	}
	defer log.Close()
	return log.Error(startupEventID, message)
}
