package elasticloadbalancing

import (
	"context"
	"strings"
	"testing"

	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	"github.com/overmindtech/aws-source/sources"
)

func TestELBMapping(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		lbName := "lbName"
		lb := ExpandedELB{
			LoadBalancerDescription: types.LoadBalancerDescription{
				LoadBalancerName: &lbName,
			},
			Instances: []types.InstanceState{},
		}

		item, err := mapELBv1ToItem(&lb, "foo.bar")
		if err != nil {
			t.Fatal(err)
		}
		if item == nil {
			t.Error("item is nil")
		}
	})
	t.Run("name check", func(t *testing.T) {
		lbName := ""
		lb := ExpandedELB{
			LoadBalancerDescription: types.LoadBalancerDescription{
				LoadBalancerName: &lbName,
			},
			Instances: []types.InstanceState{},
		}

		_, err := mapELBv1ToItem(&lb, "foo.bar")
		if err == nil {
			t.Fatal("didn't get expected error")
		}
	})
	t.Run("with hostedzone", func(t *testing.T) {
		lbName := "lbName"
		hostedZoneId := "hostedZoneId"
		lb := ExpandedELB{
			LoadBalancerDescription: types.LoadBalancerDescription{
				LoadBalancerName:          &lbName,
				CanonicalHostedZoneNameID: &hostedZoneId,
			},
			Instances: []types.InstanceState{},
		}

		item, err := mapELBv1ToItem(&lb, "foo.bar")
		if err != nil {
			t.Fatal(err)
		}
		if item == nil {
			t.Fatal("item is nil")
		}
		if len(item.LinkedItemRequests) != 1 {
			t.Fatalf("unexpected LinkedItemRequests: %v", item)
		}
		sources.CheckItem(t, item.LinkedItemRequests[0], "hostedzone", "hostedzone", hostedZoneId, "foo.bar")
	})
}

type fakeClient struct {
	DescribeLoadBalancersMock  func(ctx context.Context, params *elb.DescribeLoadBalancersInput, optFns ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error)
	DescribeInstanceHealthMock func(ctx context.Context, params *elb.DescribeInstanceHealthInput, optFns ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error)
}

func (m fakeClient) DescribeLoadBalancers(ctx context.Context, params *elb.DescribeLoadBalancersInput, optFns ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error) {
	return m.DescribeLoadBalancersMock(ctx, params, optFns...)
}

func (m fakeClient) DescribeInstanceHealth(ctx context.Context, params *elb.DescribeInstanceHealthInput, optFns ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error) {
	return m.DescribeInstanceHealthMock(ctx, params, optFns...)
}

func createFakeClient(t *testing.T) fakeClient {
	clientCalls := 0
	return fakeClient{
		DescribeLoadBalancersMock: func(ctx context.Context, params *elb.DescribeLoadBalancersInput, optFns ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error) {
			if clientCalls > 2 {
				t.Error("Called DescribeLoadBalancersMock too often (>2)")
				return nil, nil
			}
			if params.Marker == nil {
				firstName := "first"
				nextMarker := "page2"
				return &elb.DescribeLoadBalancersOutput{
					LoadBalancerDescriptions: []types.LoadBalancerDescription{
						{LoadBalancerName: &firstName},
					},
					NextMarker: &nextMarker,
				}, nil
			} else if *params.Marker == "page2" {
				secondName := "second"
				return &elb.DescribeLoadBalancersOutput{
					LoadBalancerDescriptions: []types.LoadBalancerDescription{
						{LoadBalancerName: &secondName},
					},
				}, nil
			}
			return nil, nil
		},
		DescribeInstanceHealthMock: func(ctx context.Context, params *elb.DescribeInstanceHealthInput, optFns ...func(*elb.Options)) (*elb.DescribeInstanceHealthOutput, error) {
			return &elb.DescribeInstanceHealthOutput{
				InstanceStates: []types.InstanceState{},
			}, nil
		},
	}
}

func TestGet(t *testing.T) {
	t.Parallel()
	t.Run("empty (context mismatch)", func(t *testing.T) {
		src := ELBSource{}

		items, err := src.Get(context.Background(), "foo.bar", "query")
		if items != nil {
			t.Fatalf("unexpected items: %v", items)
		}
		if err == nil {
			t.Fatalf("expected err, got nil")
		}
		if !strings.HasPrefix(err.Error(), "requested context foo.bar does not match source context .") {
			t.Errorf("expected 'requested context foo.bar does not match source context .', got '%v'", err.Error())
		}
	})
}

func TestGetImpl(t *testing.T) {
	t.Parallel()
	t.Run("with client", func(t *testing.T) {
		item, err := getImpl(context.Background(), createFakeClient(t), "foo.bar", "query")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if item == nil {
			t.Fatalf("item is nil")
		}
		if item.Attributes.AttrStruct.Fields["name"].GetStringValue() != "first" {
			t.Errorf("unexpected first item: %v", item)
		}
	})
}

func TestFind(t *testing.T) {
	t.Parallel()
	t.Run("empty (context mismatch)", func(t *testing.T) {
		src := ELBSource{}

		items, err := src.Find(context.Background(), "foo.bar")
		if items != nil {
			t.Fatalf("unexpected items: %v", items)
		}
		if err == nil {
			t.Fatalf("expected err, got nil")
		}
		if !strings.HasPrefix(err.Error(), "requested context foo.bar does not match source context .") {
			t.Errorf("expected 'requested context foo.bar does not match source context .', got '%v'", err.Error())
		}
	})
}

func TestFindImpl(t *testing.T) {
	t.Parallel()
	t.Run("with client", func(t *testing.T) {
		items, err := findImpl(context.Background(), createFakeClient(t), "foo.bar")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("unexpected items (len=%v): %v", len(items), items)
		}
		if items[0].Attributes.AttrStruct.Fields["name"].GetStringValue() != "first" {
			t.Errorf("unexpected first item: %v", items[0])
		}
		if items[1].Attributes.AttrStruct.Fields["name"].GetStringValue() != "second" {
			t.Errorf("unexpected second item: %v", items[0])
		}
	})
}
