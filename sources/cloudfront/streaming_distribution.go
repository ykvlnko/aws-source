package cloudfront

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

func streamingDistributionGetFunc(ctx context.Context, client CloudFrontClient, scope string, input *cloudfront.GetStreamingDistributionInput) (*sdp.Item, error) {
	out, err := client.GetStreamingDistribution(ctx, input)

	if err != nil {
		return nil, err
	}

	d := out.StreamingDistribution

	if d == nil {
		return nil, &sdp.QueryError{
			ErrorType:   sdp.QueryError_NOTFOUND,
			ErrorString: "streaming distribution was nil",
		}
	}

	var tags map[string]string

	// Get the tags
	tagsOut, err := client.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{
		Resource: d.ARN,
	})

	if err == nil {
		tags = tagsToMap(tagsOut.Tags)
	} else {
		tags = sources.HandleTagsError(ctx, err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get tags for streaming distribution %v: %w", *d.Id, err)
	}

	attributes, err := sources.ToAttributesCase(d)

	if err != nil {
		return nil, err
	}

	item := sdp.Item{
		Type:            "cloudfront-streaming-distribution",
		UniqueAttribute: "id",
		Attributes:      attributes,
		Scope:           scope,
		Tags:            tags,
	}

	if d.Status != nil {
		switch *d.Status {
		case "InProgress":
			item.Health = sdp.Health_HEALTH_PENDING.Enum()
		case "Deployed":
			item.Health = sdp.Health_HEALTH_OK.Enum()
		}
	}

	if d.DomainName != nil {
		// +overmind:link dns
		item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
			Query: &sdp.Query{
				Type:   "dns",
				Method: sdp.QueryMethod_SEARCH,
				Query:  *d.DomainName,
				Scope:  "global",
			},
			BlastPropagation: &sdp.BlastPropagation{
				// DNS is always linked
				In:  true,
				Out: true,
			},
		})
	}

	if dc := d.StreamingDistributionConfig; dc != nil {
		if dc.S3Origin != nil {
			if dc.S3Origin.DomainName != nil {
				// +overmind:link dns
				item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
					Query: &sdp.Query{
						Type:   "dns",
						Method: sdp.QueryMethod_SEARCH,
						Query:  *dc.S3Origin.DomainName,
						Scope:  "global",
					},
					BlastPropagation: &sdp.BlastPropagation{
						// Tightly linked
						In:  true,
						Out: true,
					},
				})
			}

			if dc.S3Origin.OriginAccessIdentity != nil {
				// +overmind:link cloudfront-cloud-front-origin-access-identity
				item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
					Query: &sdp.Query{
						Type:   "cloudfront-cloud-front-origin-access-identity",
						Method: sdp.QueryMethod_GET,
						Query:  *dc.S3Origin.OriginAccessIdentity,
						Scope:  scope,
					},
					BlastPropagation: &sdp.BlastPropagation{
						// Changing the access identity will affect the distribution
						In: true,
						// The distribution won't affect the access identity
						Out: false,
					},
				})
			}
		}

		if dc.Aliases != nil {
			for _, alias := range dc.Aliases.Items {
				// +overmind:link dns
				item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
					Query: &sdp.Query{
						Type:   "dns",
						Method: sdp.QueryMethod_SEARCH,
						Query:  alias,
						Scope:  "global",
					},
					BlastPropagation: &sdp.BlastPropagation{
						// Tightly linked
						In:  true,
						Out: true,
					},
				})
			}
		}

		if dc.Logging != nil && dc.Logging.Bucket != nil {
			// +overmind:link dns
			item.LinkedItemQueries = append(item.LinkedItemQueries, &sdp.LinkedItemQuery{
				Query: &sdp.Query{
					Type:   "dns",
					Method: sdp.QueryMethod_SEARCH,
					Query:  *dc.Logging.Bucket,
					Scope:  "global",
				},
				BlastPropagation: &sdp.BlastPropagation{
					// Tightly linked
					In:  true,
					Out: true,
				},
			})
		}
	}

	return &item, nil
}

//go:generate docgen ../../docs-data
// +overmind:type cloudfront-streaming-distribution
// +overmind:descriptiveType CloudFront Streaming Distribution
// +overmind:get Get Streaming Distribution by
// +overmind:list
// +overmind:search
// +overmind:group AWS
// +overmind:terraform:queryMap aws_cloudfront_Streamingdistribution.arn
// +overmind:terraform:method SEARCH

func NewStreamingDistributionSource(config aws.Config, accountID string) *sources.AlwaysGetSource[*cloudfront.ListStreamingDistributionsInput, *cloudfront.ListStreamingDistributionsOutput, *cloudfront.GetStreamingDistributionInput, *cloudfront.GetStreamingDistributionOutput, CloudFrontClient, *cloudfront.Options] {
	return &sources.AlwaysGetSource[*cloudfront.ListStreamingDistributionsInput, *cloudfront.ListStreamingDistributionsOutput, *cloudfront.GetStreamingDistributionInput, *cloudfront.GetStreamingDistributionOutput, CloudFrontClient, *cloudfront.Options]{
		ItemType:  "cloudfront-streaming-distribution",
		Client:    cloudfront.NewFromConfig(config),
		AccountID: accountID,
		Region:    "", // Cloudfront resources aren't tied to a region
		ListInput: &cloudfront.ListStreamingDistributionsInput{},
		ListFuncPaginatorBuilder: func(client CloudFrontClient, input *cloudfront.ListStreamingDistributionsInput) sources.Paginator[*cloudfront.ListStreamingDistributionsOutput, *cloudfront.Options] {
			return cloudfront.NewListStreamingDistributionsPaginator(client, input)
		},
		GetInputMapper: func(scope, query string) *cloudfront.GetStreamingDistributionInput {
			return &cloudfront.GetStreamingDistributionInput{
				Id: &query,
			}
		},
		ListFuncOutputMapper: func(output *cloudfront.ListStreamingDistributionsOutput, input *cloudfront.ListStreamingDistributionsInput) ([]*cloudfront.GetStreamingDistributionInput, error) {
			var inputs []*cloudfront.GetStreamingDistributionInput

			for _, sd := range output.StreamingDistributionList.Items {
				inputs = append(inputs, &cloudfront.GetStreamingDistributionInput{
					Id: sd.Id,
				})
			}

			return inputs, nil
		},
		GetFunc: streamingDistributionGetFunc,
	}
}
