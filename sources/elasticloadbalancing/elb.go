package elasticloadbalancing

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	"github.com/overmindtech/sdp-go"
)

type ELBSource struct {
	// Config AWS Config including region and credentials
	Config aws.Config

	// AccountID The id of the account that is being used. This is used by
	// sources as the first element in the context
	AccountID string

	// client The AWS client to use when making requests
	client        *elb.Client
	clientCreated bool
	clientMutex   sync.Mutex
}

func (s *ELBSource) Client() *elb.Client {
	s.clientMutex.Lock()
	defer s.clientMutex.Unlock()

	// If the client already exists then return it
	if s.clientCreated {
		return s.client
	}

	// Otherwise create a new client from the config
	s.client = elb.NewFromConfig(s.Config)
	s.clientCreated = true

	return s.client
}

// Type The type of items that this source is capable of finding
func (s *ELBSource) Type() string {
	return "elasticloadbalancing-loadbalancer-v1"
}

// Descriptive name for the source, used in logging and metadata
func (s *ELBSource) Name() string {
	return "elasticloadbalancing-aws-source"
}

// List of contexts that this source is capable of find items for. This will be
// in the format {accountID}.{region}
func (s *ELBSource) Contexts() []string {
	return []string{
		fmt.Sprintf("%v.%v", s.AccountID, s.Config.Region),
	}
}

// ELBClient Collects all functions this code uses from the AWS SDK, for test replacement.
type ELBClient interface {
	DescribeLoadBalancers(ctx context.Context, params *elb.DescribeLoadBalancersInput, optFns ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error)
	DescribeInstanceHealth(ctx context.Context, params *elb.DescribeInstanceHealthInput, optFns ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error)
}

// Get Get a single item with a given context and query. The item returned
// should have a UniqueAttributeValue that matches the `query` parameter. The
// ctx parameter contains a golang context object which should be used to allow
// this source to timeout or be cancelled when executing potentially
// long-running actions
func (s *ELBSource) Get(ctx context.Context, itemContext string, query string) (*sdp.Item, error) {
	if itemContext != s.Contexts()[0] {
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_NOCONTEXT,
			ErrorString: fmt.Sprintf("requested context %v does not match source context %v", itemContext, s.Contexts()[0]),
			Context:     itemContext,
		}
	}

	return getImpl(ctx, s.Client(), itemContext, query)
}

func getImpl(ctx context.Context, client ELBClient, itemContext string, query string) (*sdp.Item, error) {
	lbs, err := client.DescribeLoadBalancers(
		ctx,
		&elb.DescribeLoadBalancersInput{
			LoadBalancerNames: []string{
				query,
			},
		},
	)

	if err != nil {
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_OTHER,
			ErrorString: err.Error(),
			Context:     itemContext,
		}
	}

	switch len(lbs.LoadBalancerDescriptions) {
	case 0:
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_NOTFOUND,
			ErrorString: "elasticloadbalancing-loadbalancer-v1 not found",
			Context:     itemContext,
		}
	case 1:
		expanded, err := ExpandLB(ctx, client, lbs.LoadBalancerDescriptions[0])

		if err != nil {
			return nil, &sdp.ItemRequestError{
				ErrorType:   sdp.ItemRequestError_OTHER,
				ErrorString: err.Error(),
				Context:     itemContext,
			}
		}

		return mapELBv1ToItem(expanded, itemContext)
	default:
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_OTHER,
			ErrorString: fmt.Sprintf("more than 1 elasticloadbalancing-loadbalancer-v1 found, found: %v", len(lbs.LoadBalancerDescriptions)),
			Context:     itemContext,
		}
	}
}

// Find Finds all items in a given context
func (s *ELBSource) Find(ctx context.Context, itemContext string) ([]*sdp.Item, error) {
	if itemContext != s.Contexts()[0] {
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_NOCONTEXT,
			ErrorString: fmt.Sprintf("requested context %v does not match source context %v", itemContext, s.Contexts()[0]),
			Context:     itemContext,
		}
	}

	return findImpl(ctx, s.Client(), itemContext)
}

func findImpl(ctx context.Context, client ELBClient, itemContext string) ([]*sdp.Item, error) {
	items := make([]*sdp.Item, 0)
	var maxResults int32 = 100
	var nextToken *string

	for morePages := true; morePages; {
		lbs, err := client.DescribeLoadBalancers(
			ctx,
			&elb.DescribeLoadBalancersInput{
				Marker:   nextToken,
				PageSize: &maxResults,
			})

		if err != nil {
			return items, &sdp.ItemRequestError{
				ErrorType:   sdp.ItemRequestError_OTHER,
				ErrorString: err.Error(),
				Context:     itemContext,
			}
		}

		for _, lb := range lbs.LoadBalancerDescriptions {
			expanded, err := ExpandLB(ctx, client, lb)

			if err != nil {
				return nil, &sdp.ItemRequestError{
					ErrorType:   sdp.ItemRequestError_OTHER,
					ErrorString: err.Error(),
					Context:     itemContext,
				}
			}

			item, err := mapELBv1ToItem(expanded, itemContext)

			if err == nil {
				items = append(items, item)
			}
		}

		// If there is more data we should store the token so that we can use
		// that. We also need to set morePages to true so that the loop runs
		// again
		nextToken = lbs.NextMarker
		morePages = (nextToken != nil)
	}
	return items, nil
}

// Weight Returns the priority weighting of items returned by this source.
// This is used to resolve conflicts where two sources of the same type
// return an item for a GET request. In this instance only one item can be
// seen on, so the one with the higher weight value will win.
func (s *ELBSource) Weight() int {
	return 100
}

type ExpandedELB struct {
	types.LoadBalancerDescription

	// Store instance state instead of just the name
	Instances []types.InstanceState
}

func ExpandLB(ctx context.Context, client ELBClient, lb types.LoadBalancerDescription) (*ExpandedELB, error) {
	expandedLb := ExpandedELB{
		LoadBalancerDescription: lb,
	}

	instanceHealthOutput, err := client.DescribeInstanceHealth(
		ctx,
		&elb.DescribeInstanceHealthInput{
			LoadBalancerName: lb.LoadBalancerName,
		},
	)

	if err != nil {
		return nil, err
	}

	expandedLb.Instances = instanceHealthOutput.InstanceStates

	return &expandedLb, nil
}

// mapELBv1ToItem Maps a load balancer to an item
func mapELBv1ToItem(lb *ExpandedELB, itemContext string) (*sdp.Item, error) {

	if lb.LoadBalancerName == nil || *lb.LoadBalancerName == "" {
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_OTHER,
			ErrorString: "elasticloadbalancing-loadbalancer-v1 was returned with an empty name",
			Context:     itemContext,
		}
	}

	item := sdp.Item{
		Type:            "elasticloadbalancing-loadbalancer-v1",
		UniqueAttribute: "name",
		Context:         itemContext,
	}

	attrMap := make(map[string]interface{})
	attrMap["name"] = lb.LoadBalancerName
	attrMap["availabilityZones"] = lb.AvailabilityZones
	attrMap["backendServerDescriptions"] = lb.BackendServerDescriptions
	attrMap["instances"] = lb.Instances
	attrMap["listenerDescriptions"] = lb.ListenerDescriptions
	attrMap["subnets"] = lb.Subnets
	attrMap["securityGroups"] = lb.SecurityGroups
	attrMap["instances"] = lb.Instances
	attrMap["healthCheck"] = lb.HealthCheck
	attrMap["policies"] = lb.Policies
	attrMap["scheme"] = lb.Scheme
	attrMap["sourceSecurityGroup"] = lb.SourceSecurityGroup
	attrMap["VPCId"] = lb.VPCId
	attrMap["canonicalHostedZoneName"] = lb.CanonicalHostedZoneName

	if lb.CanonicalHostedZoneNameID != nil {
		attrMap["canonicalHostedZoneNameID"] = lb.CanonicalHostedZoneNameID

		item.LinkedItemRequests = append(item.LinkedItemRequests, &sdp.ItemRequest{
			Type:    "hostedzone",
			Method:  sdp.RequestMethod_GET,
			Query:   *lb.CanonicalHostedZoneNameID,
			Context: itemContext,
		})
	}

	if lb.CreatedTime != nil {
		attrMap["createdTime"] = lb.CreatedTime.String()
	}

	if lb.DNSName != nil {
		attrMap["DNSName"] = lb.DNSName

		item.LinkedItemRequests = append(item.LinkedItemRequests, &sdp.ItemRequest{
			Type:    "dns",
			Method:  sdp.RequestMethod_GET,
			Query:   *lb.DNSName,
			Context: "global",
		})
	}

	if lb.VPCId != nil {
		attrMap["VPCId"] = lb.VPCId

		item.LinkedItemRequests = append(item.LinkedItemRequests, &sdp.ItemRequest{
			Type:    "ec2-vpc",
			Method:  sdp.RequestMethod_GET,
			Query:   *lb.VPCId,
			Context: itemContext,
		})
	}

	// Security groups
	for _, group := range lb.SecurityGroups {
		item.LinkedItemRequests = append(item.LinkedItemRequests, &sdp.ItemRequest{
			Type:    "ec2-securitygroup",
			Method:  sdp.RequestMethod_GET,
			Query:   group,
			Context: itemContext,
		})
	}

	attributes, err := sdp.ToAttributes(attrMap)

	if err != nil {
		return nil, &sdp.ItemRequestError{
			ErrorType:   sdp.ItemRequestError_OTHER,
			ErrorString: fmt.Sprintf("error creating attributes: %v", err),
			Context:     itemContext,
		}
	}

	item.Attributes = attributes

	return &item, nil
}
