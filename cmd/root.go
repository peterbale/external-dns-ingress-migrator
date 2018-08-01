package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"
)

const (
	defaultDebugLogging        = false
	defaultDryRun              = false
	defaultAWSRegion           = "eu-west-1"
	defaultChangeBatchSize     = 50
	defaultChangeBatchInterval = time.Minute
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "external-dns-ingress-migrator",
	Short: "Custom actions to run again route53",
}

var (
	debugLogging        bool
	dryRun              bool
	route53Zone         string
	awsRegion           string
	changeBatchSize     int
	changeBatchInterval time.Duration
)

func init() {
	cobra.OnInitialize(initLogs)
	RootCmd.PersistentFlags().BoolVarP(&debugLogging, "debug", "X", defaultDebugLogging, "enable debug logging")
	RootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", defaultDryRun, "execute a dry run")
	RootCmd.PersistentFlags().StringVar(&route53Zone, "route53-zone", "", "set aws zone id")
	RootCmd.PersistentFlags().StringVar(&awsRegion, "aws-region", defaultAWSRegion, "set aws region")
	RootCmd.PersistentFlags().IntVarP(&changeBatchSize, "change-batch-size", "s", defaultChangeBatchSize, "set "+
		"the number of aws route53 changes made per batch")
	RootCmd.PersistentFlags().DurationVarP(&changeBatchInterval, "change-batch-interval", "i",
		defaultChangeBatchInterval, "set the interval between each aws route53 batch")
}

func initLogs() {
	if debugLogging {
		log.SetLevel(log.DebugLevel)
	}
}

func checkRequired(value, flagName string) {
	if value == "" {
		log.Fatalf("The %s value is required", flagName)
	}
}
