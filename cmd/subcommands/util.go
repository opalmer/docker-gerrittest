package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/opalmer/gerrittest"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var exit = true // Set in tests

func exitIf(flag string, err error) bool {
	if err == nil {
		return false
	}
	log.WithFields(log.Fields{
		"phase": "parse",
		"flag":  flag,
	}).WithError(err)
	if exit {
		os.Exit(1)
	}
	return true
}

func getBool(cmd *cobra.Command, flag string) bool {
	value, err := cmd.Flags().GetBool(flag)
	exitIf(flag, err)
	return value
}

func getString(cmd *cobra.Command, flag string) string {
	value, err := cmd.Flags().GetString(flag)
	exitIf(flag, err)
	return value
}

func getDuration(cmd *cobra.Command, flag string) time.Duration {
	value, err := cmd.Flags().GetDuration(flag)
	exitIf(flag, err)
	return value
}

func getUInt16(cmd *cobra.Command, flag string) uint16 {
	value, err := cmd.Flags().GetUint16(flag)
	exitIf(flag, err)
	return value
}

func jsonOutput(cmd *cobra.Command, gerrit *gerrittest.Gerrit) error {
	path := getString(cmd, "json")
	if path == "" {
		data, err := json.MarshalIndent(gerrit, "", "  ")
		fmt.Println(string(data))
		return err
	}

	return gerrit.WriteJSONFile(path)
}

func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().String(
		"log-level", "panic",
		"Configures the logging level")
	cmd.Flags().StringP(
		"json", "j", "",
		"The location to write information about the service to. Any "+
			"existing content will be overwritten.")
}
