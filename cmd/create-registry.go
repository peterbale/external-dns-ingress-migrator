package cmd

import (
	"path/filepath"

	"github.com/peterbale/external-dns-ingress-migrator/k8s"
	"github.com/peterbale/external-dns-ingress-migrator/route53"
	"github.com/spf13/cobra"
	"k8s.io/client-go/util/homedir"

	log "github.com/sirupsen/logrus"
)

var createRegistryCmd = &cobra.Command{
	Use:   "create-registry",
	Short: "Create an external-dns regisry.",
	Long: "Allows a user to create an external-dns registry within an existing route53 zone based on" +
		"any given kubernetes cluster's resources.",
	PersistentPreRun: checkClientParams,
	Run:              createRegistry,
}

var (
	kubeconfig        string
	context           string
	externalDNSPrefix string
	externalDNSOwner  string
)

func init() {
	RootCmd.AddCommand(createRegistryCmd)
	if home := homedir.HomeDir(); home != "" {
		RootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"),
			"(optional) set absolute path to kubeconfig file")
	} else {
		RootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "set absolute path to "+
			"kubeconfig file")
	}
	RootCmd.PersistentFlags().StringVar(&context, "context", "", "set kubernetes context to use (must be a valid"+
		"dns domain)")
	RootCmd.PersistentFlags().StringVar(&externalDNSPrefix, "external-dns-prefix", "",
		"override default external-dns prefix to create")
	RootCmd.PersistentFlags().StringVar(&externalDNSOwner, "external-dns-owner", "",
		"override default external-dns owner to use")
}

func createRegistry(cmd *cobra.Command, args []string) {
	k8sClient, err := k8s.CreateClient(k8s.Config{
		Kubeconfig: &kubeconfig,
		Context:    &context,
	})
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}
	registryData, err := k8sClient.GetRegistryData()
	if err != nil {
		log.Fatalf("Failed to get registry data: %v", err)
	}

	if err := route53.CreateClient(route53.Config{
		DryRun:              &dryRun,
		Region:              &awsRegion,
		Zone:                &route53Zone,
		ChangeBatchSize:     &changeBatchSize,
		ChangeBatchInterval: &changeBatchInterval,
		ExternalDNSOwner:    &externalDNSOwner,
		ExternalDNSPrefix:   &externalDNSPrefix,
	}).CreateRegistry(registryData); err != nil {
		log.Fatalf("Failed to create registry: %v", err)
	}
}

func checkClientParams(cmd *cobra.Command, args []string) {
	checkRequired(externalDNSOwner, "external-dns-owner")
	checkRequired(externalDNSPrefix, "external-dns-prefix")
	checkRequired(kubeconfig, "kubeconfig")
	checkRequired(context, "context")
	checkRequired(route53Zone, "route53-zone")
}
