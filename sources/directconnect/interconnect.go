package directconnect

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/directconnect/types"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/directconnect"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

func interconnectOutputMapper(_ context.Context, _ *directconnect.Client, scope string, _ *directconnect.DescribeInterconnectsInput, output *directconnect.DescribeInterconnectsOutput) ([]*sdp.Item, error) {
	items := make([]*sdp.Item, 0)

	for _, interconnect := range output.Interconnects {
		attributes, err := sources.ToAttributesCase(interconnect, "tags")
		if err != nil {
			return nil, err
		}

		item := sdp.Item{
			Type:            "directconnect-interconnect",
			UniqueAttribute: "interconnectId",
			Attributes:      attributes,
			Scope:           scope,
			Tags:            tagsToMap(interconnect.Tags),
		}

		switch interconnect.InterconnectState {
		case types.InterconnectStateRequested:
			item.Health = sdp.Health_HEALTH_PENDING.Enum()
		case types.InterconnectStatePending:
			item.Health = sdp.Health_HEALTH_PENDING.Enum()
		case types.InterconnectStateAvailable:
			item.Health = sdp.Health_HEALTH_OK.Enum()
		case types.InterconnectStateDown:
			item.Health = sdp.Health_HEALTH_ERROR.Enum()
		case types.InterconnectStateDeleting:
			item.Health = sdp.Health_HEALTH_UNKNOWN.Enum()
		case types.InterconnectStateDeleted:
			item.Health = sdp.Health_HEALTH_UNKNOWN.Enum()
		case types.InterconnectStateUnknown:
			item.Health = sdp.Health_HEALTH_UNKNOWN.Enum()
		}

		if interconnect.InterconnectId != nil {
			// +overmind:link directconnect-hosted-connection
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "directconnect-hosted-connection",
					Method: sdp.QueryMethod_SEARCH,
					Query:  *interconnect.InterconnectId,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Interconnect and hosted connections are tightly coupled
					// Changing one will affect the other
					In:  true,
					Out: true,
				},
			})
		}

		if interconnect.LagId != nil {
			// +overmind:link directconnect-lag
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "directconnect-lag",
					Method: sdp.QueryMethod_GET,
					Query:  *interconnect.LagId,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Interconnect and LAG are tightly coupled
					// Changing one will affect the other
					In:  true,
					Out: true,
				},
			})
		}

		if interconnect.LoaIssueTime != nil {
			// +overmind:link directconnect-loa
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "directconnect-loa",
					Method: sdp.QueryMethod_GET,
					Query:  *interconnect.InterconnectId,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Changes to the loa will affect this
					In: true,
					// We can't affect the loa
					Out: false,
				},
			})
		}

		if interconnect.Location != nil {
			// +overmind:link directconnect-location
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "directconnect-location",
					Method: sdp.QueryMethod_GET,
					Query:  *interconnect.Location,
					Scope:  scope,
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Changes to the location will affect this, i.e., its speed, provider, etc.
					In: true,
					// We can't affect the location
					Out: false,
				},
			})
		}

		items = append(items, &item)
	}

	return items, nil
}

//go:generate docgen ../../docs-data
// +overmind:type directconnect-interconnect
// +overmind:descriptiveType Direct Connect Interconnect
// +overmind:get Get a Interconnect by InterconnectId
// +overmind:list List all Interconnects
// +overmind:search Search Interconnects by ARN
// +overmind:group AWS

func NewInterconnectSource(config aws.Config, accountID string, limit *sources.LimitBucket) *sources.DescribeOnlySource[*directconnect.DescribeInterconnectsInput, *directconnect.DescribeInterconnectsOutput, *directconnect.Client, *directconnect.Options] {
	return &sources.DescribeOnlySource[*directconnect.DescribeInterconnectsInput, *directconnect.DescribeInterconnectsOutput, *directconnect.Client, *directconnect.Options]{
		Config:    config,
		Client:    directconnect.NewFromConfig(config),
		AccountID: accountID,
		ItemType:  "directconnect-interconnect",
		DescribeFunc: func(ctx context.Context, client *directconnect.Client, input *directconnect.DescribeInterconnectsInput) (*directconnect.DescribeInterconnectsOutput, error) {
			limit.Wait(ctx) // Wait for rate limiting
			return client.DescribeInterconnects(ctx, input)
		},
		InputMapperGet: func(scope, query string) (*directconnect.DescribeInterconnectsInput, error) {
			return &directconnect.DescribeInterconnectsInput{
				InterconnectId: &query,
			}, nil
		},
		InputMapperList: func(scope string) (*directconnect.DescribeInterconnectsInput, error) {
			return &directconnect.DescribeInterconnectsInput{}, nil
		},
		OutputMapper: interconnectOutputMapper,
	}
}
