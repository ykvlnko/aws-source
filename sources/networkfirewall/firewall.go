package networkfirewall

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

type unifiedFirewall struct {
	Name                 string
	Properties           *types.Firewall
	Status               *types.FirewallStatus
	LoggingConfiguration *types.LoggingConfiguration
	ResourcePolicy       *string
}

func firewallGetFunc(ctx context.Context, client networkFirewallClient, scope string, input *networkfirewall.DescribeFirewallInput) (*sdp.Item, error) {
	response, err := client.DescribeFirewall(ctx, input)

	if err != nil {
		return nil, err
	}

	if response == nil || response.Firewall == nil || response.Firewall.FirewallName == nil {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOTFOUND,
			ErrorString: "Firewall was nil",
			Scope:       scope,
		}
	}

	uf := unifiedFirewall{
		Name:       *response.Firewall.FirewallName,
		Properties: response.Firewall,
		Status:     response.FirewallStatus,
	}

	// Enrich with more info
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		resp, _ := client.DescribeLoggingConfiguration(ctx, &networkfirewall.DescribeLoggingConfigurationInput{
			FirewallArn: response.Firewall.FirewallArn,
		})

		if resp != nil {
			uf.LoggingConfiguration = resp.LoggingConfiguration
		}
	}()
	go func() {
		defer wg.Done()
		resp, _ := client.DescribeResourcePolicy(ctx, &networkfirewall.DescribeResourcePolicyInput{
			ResourceArn: response.Firewall.FirewallArn,
		})

		if resp != nil {
			uf.ResourcePolicy = resp.Policy
		}
	}()

	wg.Wait()

	attributes, err := sources.ToAttributesCase(uf)

	if err != nil {
		return nil, err
	}

	var health *sdp.Health

	if response.FirewallStatus != nil {
		switch response.FirewallStatus.Status {
		case types.FirewallStatusValueDeleting:
			health = sdp.Health_HEALTH_PENDING.Enum()
		case types.FirewallStatusValueProvisioning:
			health = sdp.Health_HEALTH_PENDING.Enum()
		case types.FirewallStatusValueReady:
			health = sdp.Health_HEALTH_OK.Enum()
		}
	}

	tags := make(map[string]string)

	for _, tag := range response.Firewall.Tags {
		tags[*tag.Key] = *tag.Value
	}

	item := sdp.Item{
		Type:            "network-firewall-firewall",
		UniqueAttribute: "name",
		Scope:           scope,
		Attributes:      attributes,
		Health:          health,
		Tags:            tags,
	}

	config := response.Firewall

	if uf.LoggingConfiguration != nil {
		for _, config := range uf.LoggingConfiguration.LogDestinationConfigs {
			switch config.LogDestinationType {
			case types.LogDestinationTypeCloudwatchLogs:
				logGroup, ok := config.LogDestination["logGroup"]

				if ok {
					// +overmind:link logs-log-group
					item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
						Query: &sdp.Query{
							Type:   "logs-log-group",
							Method: sdp.QueryMethod_GET,
							Query:  logGroup,
							Scope:  scope,
						},
						BlastPropagation: &sdp.BlastPropagation{
							In:  false,
							Out: true,
						},
					})
				}
			case types.LogDestinationTypeS3:
				bucketName, ok := config.LogDestination["bucketName"]

				if ok {
					//+overmind:link s3-bucket
					item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
						Query: &sdp.Query{
							Type:   "s3-bucket",
							Method: sdp.QueryMethod_GET,
							Query:  bucketName,
							Scope:  scope,
						},
						BlastPropagation: &sdp.BlastPropagation{
							In:  false,
							Out: true,
						},
					})
				}
			case types.LogDestinationTypeKinesisDataFirehose:
				deliveryStream, ok := config.LogDestination["deliveryStream"]

				if ok {
					//+overmind:link firehose-delivery-stream
					item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
						Query: &sdp.Query{
							Type:   "firehose-delivery-stream",
							Method: sdp.QueryMethod_GET,
							Query:  deliveryStream,
							Scope:  scope,
						},
						BlastPropagation: &sdp.BlastPropagation{
							In:  false,
							Out: true,
						},
					})
				}
			}
		}
	}

	if uf.ResourcePolicy != nil {
		//+overmind:link iam-policy
		item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
			Query: &sdp.Query{
				Type:   "iam-policy",
				Method: sdp.QueryMethod_GET,
				Query:  *uf.ResourcePolicy,
				Scope:  scope,
			},
			BlastPropagation: &sdp.BlastPropagation{
				In:  true,
				Out: false,
			},
		})
	}

	if config.FirewallPolicyArn != nil {
		if a, err := sources.ParseARN(*config.FirewallPolicyArn); err == nil {
			//+overmind:link network-firewall-firewall-policy
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "network-firewall-firewall-policy",
					Method: sdp.QueryMethod_SEARCH,
					Query:  *config.FirewallPolicyArn,
					Scope:  sources.FormatScope(a.AccountID, a.Region),
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Policy will affect the firewall but not the other way around
					In:  true,
					Out: false,
				},
			})
		}
	}

	for _, mapping := range config.SubnetMappings {
		if mapping.SubnetId != nil {
			//+overmind:link ec2-subnet
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "ec2-subnet",
					Method: sdp.QueryMethod_GET,
					Query:  *mapping.SubnetId,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Changes to public subnets could affect the firewall
					In:  true,
					Out: false,
				},
			})
		}
	}

	if config.VpcId != nil {
		//+overmind:link ec2-vpc
		item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
			Query: &sdp.Query{
				Type:   "ec2-vpc",
				Method: sdp.QueryMethod_GET,
				Query:  *config.VpcId,
				Scope:  scope,
			},
			BlastPropagation: &sdp.BlastPropagation{
				// Changes to the VPC could affect the firewall
				In:  true,
				Out: false,
			},
		})
	}

	//+overmind:link kms-key
	item.LinkedItemQueries = append(item.LinkedItemQueries, encryptionConfigurationLink(config.EncryptionConfiguration, scope))

	for _, state := range response.FirewallStatus.SyncStates {
		if state.Attachment != nil && state.Attachment.SubnetId != nil {
			//+overmind:link ec2-subnet
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "ec2-subnet",
					Method: sdp.QueryMethod_GET,
					Query:  *state.Attachment.SubnetId,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Changes to public subnets could affect the firewall
					In:  true,
					Out: false,
				},
			})
		}
	}

	return &item, nil
}

//go:generate docgen ../../docs-data
// +overmind:type network-firewall-firewall
// +overmind:descriptiveType Network Firewall
// +overmind:get Get a Network Firewall by name
// +overmind:list List Network Firewalls
// +overmind:search Search for Network Firewalls by ARN
// +overmind:group AWS
// +overmind:terraform:queryMap aws_networkfirewall_firewall.name

func NewFirewallSource(config aws.Config, accountID string, region string) *sources.AlwaysGetSource[*networkfirewall.ListFirewallsInput, *networkfirewall.ListFirewallsOutput, *networkfirewall.DescribeFirewallInput, *networkfirewall.DescribeFirewallOutput, networkFirewallClient, *networkfirewall.Options] {
	return &sources.AlwaysGetSource[*networkfirewall.ListFirewallsInput, *networkfirewall.ListFirewallsOutput, *networkfirewall.DescribeFirewallInput, *networkfirewall.DescribeFirewallOutput, networkFirewallClient, *networkfirewall.Options]{
		ItemType:  "network-firewall-firewall",
		Client:    networkfirewall.NewFromConfig(config),
		AccountID: accountID,
		Region:    region,
		ListInput: &networkfirewall.ListFirewallsInput{},
		GetInputMapper: func(scope, query string) *networkfirewall.DescribeFirewallInput {
			return &networkfirewall.DescribeFirewallInput{
				FirewallName: &query,
			}
		},
		SearchGetInputMapper: func(scope, query string) (*networkfirewall.DescribeFirewallInput, error) {
			return &networkfirewall.DescribeFirewallInput{
				FirewallArn: &query,
			}, nil
		},
		ListFuncPaginatorBuilder: func(client networkFirewallClient, input *networkfirewall.ListFirewallsInput) sources.Paginator[*networkfirewall.ListFirewallsOutput, *networkfirewall.Options] {
			return networkfirewall.NewListFirewallsPaginator(client, input)
		},
		ListFuncOutputMapper: func(output *networkfirewall.ListFirewallsOutput, input *networkfirewall.ListFirewallsInput) ([]*networkfirewall.DescribeFirewallInput, error) {
			var inputs []*networkfirewall.DescribeFirewallInput

			for _, firewall := range output.Firewalls {
				inputs = append(inputs, &networkfirewall.DescribeFirewallInput{
					FirewallArn: firewall.FirewallArn,
				})
			}
			return inputs, nil
		},
		GetFunc: func(ctx context.Context, client networkFirewallClient, scope string, input *networkfirewall.DescribeFirewallInput) (*sdp.Item, error) {
			return firewallGetFunc(ctx, client, scope, input)
		},
	}
}
