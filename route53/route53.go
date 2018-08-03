package route53

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/peterbale/external-dns-ingress-migrator/registry"

	aws_route53 "github.com/aws/aws-sdk-go/service/route53"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	DryRun              *bool
	Region              *string
	Zone                *string
	ChangeBatchSize     *int
	ChangeBatchInterval *time.Duration
	ExternalDNSOwner    *string
	ExternalDNSPrefix   *string
}

type Route53 interface {
	CreateRegistry(registryData []registry.Data) error
}

type route53 struct {
	dryRun              *bool
	zone                *string
	changeBatchSize     *int
	changeBatchInterval *time.Duration
	externalDNSOwner    *string
	externalDNSPrefix   *string
	client              *aws_route53.Route53
}

func CreateClient(cnf Config) Route53 {
	sess := session.Must(session.NewSession())
	return &route53{
		dryRun:              cnf.DryRun,
		zone:                cnf.Zone,
		changeBatchInterval: cnf.ChangeBatchInterval,
		changeBatchSize:     cnf.ChangeBatchSize,
		externalDNSOwner:    cnf.ExternalDNSOwner,
		externalDNSPrefix:   cnf.ExternalDNSPrefix,
		client:              aws_route53.New(sess, &aws.Config{Region: cnf.Region}),
	}
}

func (r *route53) CreateRegistry(registryData []registry.Data) error {
	existingRecordSets, err := r.getRecordSets()
	if err != nil {
		return fmt.Errorf("failed to get existing record sets for \"%s\" zone: %v", *r.zone, err)
	}
	var rrsetGroups [][]map[*aws_route53.ResourceRecordSet]string
	for _, d := range registryData {
		name := *r.externalDNSPrefix + d.Hostname + "."
		if rrset, exists := existingRecordSets[name]; exists {
			var values []string
			for _, rr := range rrset.ResourceRecords {
				values = append(values, *rr.Value)
			}
			log.Debugf("record \"%s\" already exists with values: %v", name, values)
			continue
		}
		var rrsetGroup []map[*aws_route53.ResourceRecordSet]string
		rrsetAction := make(map[*aws_route53.ResourceRecordSet]string)
		value := fmt.Sprintf("\"heritage=external-dns,external-dns/owner=%s,external-dns/resource=ingress/%s/%s\"",
			*r.externalDNSOwner, d.Namespace, d.Name)
		rrsetAction[&aws_route53.ResourceRecordSet{
			Name: aws.String(name),
			Type: aws.String("TXT"),
			TTL:  aws.Int64(int64(300)),
			ResourceRecords: []*aws_route53.ResourceRecord{
				{
					Value: aws.String(value),
				},
			},
		}] = "CREATE"
		rrsetGroup = append(rrsetGroup, rrsetAction)
		rrsetGroups = append(rrsetGroups, rrsetGroup)
	}
	if len(rrsetGroups) == 0 {
		log.Info("No changes to make to route53")
		return nil
	}
	return r.changeRecordSets(rrsetGroups)
}

func (r *route53) getRecordSets() (map[string]*aws_route53.ResourceRecordSet, error) {
	rrsets := make(map[string]*aws_route53.ResourceRecordSet)

	rrsetParams := &aws_route53.ListResourceRecordSetsInput{HostedZoneId: r.zone}
	pageNum := 0
	err := r.client.ListResourceRecordSetsPages(rrsetParams,
		func(page *aws_route53.ListResourceRecordSetsOutput, lastPage bool) bool {
			pageNum++
			for _, entry := range page.ResourceRecordSets {
				log.Debugf("Found entry: %s", *entry.Name)
				rrsets[*entry.Name] = entry
			}
			return pageNum <= 30
		},
	)
	if err != nil {
		return rrsets, fmt.Errorf("failed to list route53 record set for zone \"%v\": %v", r.zone, err)
	}

	return rrsets, nil
}

func (r *route53) changeRecordSets(rrsetGroups [][]map[*aws_route53.ResourceRecordSet]string) error {
	var err error
	var recordHostnames, failedHostnames []string
	var counter int

	log.Infof("Changing route53 record set in batches of %v for zone: %s...", *r.changeBatchSize, *r.zone)

	batchRrsets := &aws_route53.ChangeResourceRecordSetsInput{
		HostedZoneId: r.zone,
		ChangeBatch:  &aws_route53.ChangeBatch{Changes: []*aws_route53.Change{}},
	}
	for _, rrsetGroup := range rrsetGroups {
		counter++
		for _, rrsets := range rrsetGroup {
			for rrset, action := range rrsets {
				batchRrsets.ChangeBatch.Changes = append(batchRrsets.ChangeBatch.Changes, &aws_route53.Change{
					Action:            &action,
					ResourceRecordSet: rrset,
				})
				recordHostnames = append(recordHostnames, *rrset.Name)
			}
		}

		if counter%*r.changeBatchSize == 0 || counter == len(rrsetGroups) {
			log.Debugf("Record set to change: %+v", batchRrsets)
			if *r.dryRun {
				log.Info("Dry run, would have performed the following...")
				for _, change := range batchRrsets.ChangeBatch.Changes {
					var values []string
					for _, rr := range change.ResourceRecordSet.ResourceRecords {
						values = append(values, *rr.Value)
					}
					log.Infof("Action: \"%s\", Hostname:\"%s\", Values: \"%v\"", *change.Action,
						*change.ResourceRecordSet.Name, values)
				}
			} else {
				if _, err = r.client.ChangeResourceRecordSets(batchRrsets); err != nil {
					failedHostnames = append(failedHostnames, recordHostnames...)
					continue
				}
				log.Infof("Changed record set: %s", recordHostnames)
			}
			batchRrsets.ChangeBatch.Changes = []*aws_route53.Change{}
			recordHostnames = []string{}
			if counter < len(rrsetGroups) {
				log.Infof("Waiting for interval: %v...", *r.changeBatchInterval)
				time.Sleep(*r.changeBatchInterval)
			}
		}
	}

	if len(failedHostnames) > 0 {
		return fmt.Errorf("failed to change all route53 record sets \"%v\": %v", failedHostnames, err)
	}
	return nil
}
