package elbv2

import (
	"context"
	"testing"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

func TestTargetGroupOutputMapper(t *testing.T) {
	output := elbv2.DescribeTargetGroupsOutput{
		TargetGroups: []types.TargetGroup{
			{
				TargetGroupArn:             sources.PtrString("arn:aws:elasticloadbalancing:eu-west-2:944651592624:targetgroup/k8s-default-apiserve-d87e8f7010/559d207158e41222"),
				TargetGroupName:            sources.PtrString("k8s-default-apiserve-d87e8f7010"),
				Protocol:                   types.ProtocolEnumHttp,
				Port:                       sources.PtrInt32(8080),
				VpcId:                      sources.PtrString("vpc-0c72199250cd479ea"), // link
				HealthCheckProtocol:        types.ProtocolEnumHttp,
				HealthCheckPort:            sources.PtrString("traffic-port"),
				HealthCheckEnabled:         sources.PtrBool(true),
				HealthCheckIntervalSeconds: sources.PtrInt32(10),
				HealthCheckTimeoutSeconds:  sources.PtrInt32(10),
				HealthyThresholdCount:      sources.PtrInt32(10),
				UnhealthyThresholdCount:    sources.PtrInt32(10),
				HealthCheckPath:            sources.PtrString("/"),
				Matcher: &types.Matcher{
					HttpCode: sources.PtrString("200"),
					GrpcCode: sources.PtrString("code"),
				},
				LoadBalancerArns: []string{
					"arn:aws:elasticloadbalancing:eu-west-2:944651592624:loadbalancer/app/ingress/1bf10920c5bd199d", // link
				},
				TargetType:      types.TargetTypeEnumIp,
				ProtocolVersion: sources.PtrString("HTTP1"),
				IpAddressType:   types.TargetGroupIpAddressTypeEnumIpv4,
			},
		},
	}

	items, err := targetGroupOutputMapper(context.Background(), nil, "foo", nil, &output)

	if err != nil {
		t.Error(err)
	}

	for _, item := range items {
		if err := item.Validate(); err != nil {
			t.Error(err)
		}
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", len(items))
	}

	item := items[0]

	// It doesn't really make sense to test anything other than the linked items
	// since the attributes are converted automatically
	tests := sources.QueryTests{
		{
			ExpectedType:   "ec2-vpc",
			ExpectedMethod: sdp.QueryMethod_GET,
			ExpectedQuery:  "vpc-0c72199250cd479ea",
			ExpectedScope:  "foo",
		},
		{
			ExpectedType:   "elbv2-load-balancer",
			ExpectedMethod: sdp.QueryMethod_SEARCH,
			ExpectedQuery:  "arn:aws:elasticloadbalancing:eu-west-2:944651592624:loadbalancer/app/ingress/1bf10920c5bd199d",
			ExpectedScope:  "944651592624.eu-west-2",
		},
	}

	tests.Execute(t, item)
}
