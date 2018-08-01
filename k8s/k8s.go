package k8s

import (
	"fmt"
	"strings"

	"github.com/peterbale/external-dns-ingress-migrator/registry"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type Config struct {
	Kubeconfig *string
	Context    *string
}

type K8s interface {
	GetRegistryData() ([]registry.Data, error)
}

type k8s struct {
	context *string
	client  *kubernetes.Clientset
}

func CreateClient(cnf Config) (K8s, error) {
	config, err := clientcmd.BuildConfigFromFlags("", *cnf.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config from file: %v", cnf.Kubeconfig)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config from file: %v", cnf.Kubeconfig)
	}
	return &k8s{
		context: cnf.Context,
		client:  client,
	}, nil
}

func (k *k8s) GetRegistryData() ([]registry.Data, error) {
	var d []registry.Data

	ingresses, err := k.client.ExtensionsV1beta1().Ingresses("").List(*new(apiv1.ListOptions))
	if err != nil {
		return d, fmt.Errorf("failed to get ingresses: %v", err)
	}

	for _, ingress := range k.getValidUniqueHostnameIngresses(ingresses.Items) {
		for _, rule := range ingress.Spec.Rules {
			log.Debugf("Creating registy data for host: %s", rule.Host)
			d = append(d, registry.Data{
				Name:      ingress.Name,
				Namespace: ingress.Namespace,
				Hostname:  rule.Host,
			})
		}
	}

	return d, nil
}

func (k *k8s) getValidUniqueHostnameIngresses(ingresses []v1beta1.Ingress) []v1beta1.Ingress {
	hostnameIngresses := make(map[string][]v1beta1.Ingress)
	var uniqueHostnameIngresses []v1beta1.Ingress
	for _, ingress := range ingresses {
		for _, rule := range ingress.Spec.Rules {
			if !strings.Contains(rule.Host, *k.context) {
				log.Warnf("Ignoring invalid domain hostname: %s", rule.Host)
				continue
			}
			hostnameIngresses[rule.Host] = append(hostnameIngresses[rule.Host], ingress)
		}
	}
	for hostname, ingresses := range hostnameIngresses {
		selectedIngress := ingresses[0]
		if len(ingresses) > 1 {
			ingressStatus := make(map[string][]string)
			var invalidIngress bool
			for _, ingress := range ingresses {
				for _, loadbalancerIngress := range ingress.Status.LoadBalancer.Ingress {
					ingressStatus["hostnames"] = append(ingressStatus["hostnames"], loadbalancerIngress.Hostname)
					ingressStatus["ips"] = append(ingressStatus["ips"], loadbalancerIngress.IP)
				}
			}
			for _, targets := range ingressStatus {
				if len(targets) > 1 {
					for _, target := range targets {
						if target != targets[0] {
							invalidIngress = true
						}
					}
				}
			}
			if invalidIngress {
				log.Warnf("Ignoring \"%s\" hostname as multiple ingresses are trying to set different targets: %v",
					hostname, ingressStatus)
				continue
			}
			log.Warnf("Duplicate ingress host found for \"%s\", using \"%s\" ingress in \"%s\" namespace",
				hostname, selectedIngress.Name, selectedIngress.Namespace)
		} else {
			log.Debugf("Hostname: \"%s\", Ingress: \"%s\", Namespace: \"&s\"", hostname, selectedIngress.Name,
				selectedIngress.Namespace)
		}
		uniqueHostnameIngresses = append(uniqueHostnameIngresses, selectedIngress)
	}
	return uniqueHostnameIngresses
}
