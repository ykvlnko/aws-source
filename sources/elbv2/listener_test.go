package elbv2

import (
	"context"
	"testing"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

func TestListenerOutputMapper(t *testing.T) {
	output := elbv2.DescribeListenersOutput{
		Listeners: []types.Listener{
			{
				ListenerArn:     sources.PtrString("arn:aws:elasticloadbalancing:eu-west-2:944651592624:listener/app/ingress/1bf10920c5bd199d/9d28f512be129134"),
				LoadBalancerArn: sources.PtrString("arn:aws:elasticloadbalancing:eu-west-2:944651592624:loadbalancer/app/ingress/1bf10920c5bd199d"), // link
				Port:            sources.PtrInt32(443),
				Protocol:        types.ProtocolEnumHttps,
				Certificates: []types.Certificate{
					{
						CertificateArn: sources.PtrString("arn:aws:acm:eu-west-2:944651592624:certificate/acd84d34-fb78-4411-bd8a-43684a3477c5"), // link
						IsDefault:      sources.PtrBool(true),
					},
				},
				SslPolicy: sources.PtrString("ELBSecurityPolicy-2016-08"),
				AlpnPolicy: []string{
					"policy1",
				},
				DefaultActions: []types.Action{
					// This is tested in actions.go
				},
			},
		},
	}

	items, err := listenerOutputMapper(context.Background(), nil, "foo", nil, &output)

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
			ExpectedType:   "elbv2-load-balancer",
			ExpectedMethod: sdp.QueryMethod_SEARCH,
			ExpectedQuery:  "arn:aws:elasticloadbalancing:eu-west-2:944651592624:loadbalancer/app/ingress/1bf10920c5bd199d",
			ExpectedScope:  "944651592624.eu-west-2",
		},
		{
			ExpectedType:   "acm-certificate",
			ExpectedMethod: sdp.QueryMethod_SEARCH,
			ExpectedQuery:  "arn:aws:acm:eu-west-2:944651592624:certificate/acd84d34-fb78-4411-bd8a-43684a3477c5",
			ExpectedScope:  "944651592624.eu-west-2",
		},
	}

	tests.Execute(t, item)
}
