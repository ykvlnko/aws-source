package networkfirewall

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

type unifiedRuleGroup struct {
	Name       string
	Properties *types.RuleGroupResponse
	RuleGroup  *types.RuleGroup
}

func ruleGroupGetFunc(ctx context.Context, client networkFirewallClient, scope string, input *networkfirewall.DescribeRuleGroupInput) (*sdp.Item, error) {
	resp, err := client.DescribeRuleGroup(ctx, input)

	if err != nil {
		return nil, err
	}

	if resp.RuleGroupResponse == nil || resp.RuleGroup == nil {
		return nil, errors.New("empty response")
	}

	urg := unifiedRuleGroup{
		Name:       *resp.RuleGroupResponse.RuleGroupName,
		Properties: resp.RuleGroupResponse,
		RuleGroup:  resp.RuleGroup,
	}

	attributes, err := sources.ToAttributesCase(urg)

	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)

	for _, tag := range resp.RuleGroupResponse.Tags {
		tags[*tag.Key] = *tag.Value
	}

	var health *sdp.Health

	switch resp.RuleGroupResponse.RuleGroupStatus {
	case types.ResourceStatusActive:
		health = sdp.Health_HEALTH_OK.Enum()
	case types.ResourceStatusDeleting:
		health = sdp.Health_HEALTH_PENDING.Enum()
	case types.ResourceStatusError:
		health = sdp.Health_HEALTH_ERROR.Enum()
	}

	item := sdp.Item{
		Type:            "network-firewall-rule-group",
		UniqueAttribute: "name",
		Attributes:      attributes,
		Scope:           scope,
		Tags:            tags,
		Health:          health,
	}

	//+overmind:link kms-key
	item.LinkedItemQueries = append(item.LinkedItemQueries, encryptionConfigurationLink(resp.RuleGroupResponse.EncryptionConfiguration, scope))

	if resp.RuleGroupResponse.SnsTopic != nil {
		if a, err := sources.ParseARN(*resp.RuleGroupResponse.SnsTopic); err == nil {
			//+overmind:link sns-topic
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "sns-topic",
					Method: sdp.QueryMethod_SEARCH,
					Query:  *resp.RuleGroupResponse.SnsTopic,
					Scope:  sources.FormatScope(a.AccountID, a.Region),
				},
				BlastPropagation: &sdp.BlastPropagation{
					In:  false,
					Out: true,
				},
			})
		}
	}

	if resp.RuleGroupResponse.SourceMetadata != nil && resp.RuleGroupResponse.SourceMetadata.SourceArn != nil {
		if a, err := sources.ParseARN(*resp.RuleGroupResponse.SourceMetadata.SourceArn); err == nil {
			//+overmind:link network-firewall-rule-group
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "network-firewall-rule-group",
					Method: sdp.QueryMethod_SEARCH,
					Query:  *resp.RuleGroupResponse.SourceMetadata.SourceArn,
					Scope:  sources.FormatScope(a.AccountID, a.Region),
				},
				BlastPropagation: &sdp.BlastPropagation{
					In:  false,
					Out: false,
				},
			})
		}
	}

	return &item, nil
}

//go:generate docgen ../../docs-data
// +overmind:type network-firewall-rule-group
// +overmind:descriptiveType Network Firewall Rule Group
// +overmind:get Get a Network Firewall Rule Group by name
// +overmind:list List Network Firewall Rule Groups
// +overmind:search Search for Network Firewall Rule Groups by ARN
// +overmind:group AWS
// +overmind:terraform:queryMap aws_networkfirewall_rule_group.name

func NewRuleGroupSource(config aws.Config, accountID string, region string) *sources.AlwaysGetSource[*networkfirewall.ListRuleGroupsInput, *networkfirewall.ListRuleGroupsOutput, *networkfirewall.DescribeRuleGroupInput, *networkfirewall.DescribeRuleGroupOutput, networkFirewallClient, *networkfirewall.Options] {
	return &sources.AlwaysGetSource[*networkfirewall.ListRuleGroupsInput, *networkfirewall.ListRuleGroupsOutput, *networkfirewall.DescribeRuleGroupInput, *networkfirewall.DescribeRuleGroupOutput, networkFirewallClient, *networkfirewall.Options]{
		ItemType:  "network-firewall-rule-group",
		Client:    networkfirewall.NewFromConfig(config),
		AccountID: accountID,
		Region:    region,
		ListInput: &networkfirewall.ListRuleGroupsInput{},
		GetInputMapper: func(scope, query string) *networkfirewall.DescribeRuleGroupInput {
			return &networkfirewall.DescribeRuleGroupInput{
				RuleGroupName: &query,
			}
		},
		SearchGetInputMapper: func(scope, query string) (*networkfirewall.DescribeRuleGroupInput, error) {
			return &networkfirewall.DescribeRuleGroupInput{
				RuleGroupArn: &query,
			}, nil
		},
		ListFuncPaginatorBuilder: func(client networkFirewallClient, input *networkfirewall.ListRuleGroupsInput) sources.Paginator[*networkfirewall.ListRuleGroupsOutput, *networkfirewall.Options] {
			return networkfirewall.NewListRuleGroupsPaginator(client, input)
		},
		ListFuncOutputMapper: func(output *networkfirewall.ListRuleGroupsOutput, input *networkfirewall.ListRuleGroupsInput) ([]*networkfirewall.DescribeRuleGroupInput, error) {
			var inputs []*networkfirewall.DescribeRuleGroupInput

			for _, rg := range output.RuleGroups {
				inputs = append(inputs, &networkfirewall.DescribeRuleGroupInput{
					RuleGroupArn: rg.Arn,
				})
			}
			return inputs, nil
		},
		GetFunc: func(ctx context.Context, client networkFirewallClient, scope string, input *networkfirewall.DescribeRuleGroupInput) (*sdp.Item, error) {
			return ruleGroupGetFunc(ctx, client, scope, input)
		},
	}
}
