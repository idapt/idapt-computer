package revoke

import (
	"log"
	"os"
)

func Trigger(configPath string) {
	log.Printf("revoke: machine row was deleted; cleaning up and exiting")
	if configPath != "" {
		_ = os.Remove(configPath)
	}
	_ = os.Remove("/var/lib/idapt/last-event-id")
	os.Exit(0)
}
