package eks

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
)

var FargateTestClient = TestClient{
	DescribeFargateProfileOutput: &eks.DescribeFargateProfileOutput{
		FargateProfile: &types.FargateProfile{
			ClusterName:         sources.PtrString("cluster"),
			CreatedAt:           sources.PtrTime(time.Now()),
			FargateProfileArn:   sources.PtrString("arn:partition:service:region:account-id:resource-type/resource-id"),
			FargateProfileName:  sources.PtrString("name"),
			PodExecutionRoleArn: sources.PtrString("arn:partition:service::account-id:resource-type/resource-id"),
			Selectors: []types.FargateProfileSelector{
				{
					Labels:    map[string]string{},
					Namespace: sources.PtrString("namespace"),
				},
			},
			Status: types.FargateProfileStatusActive,
			Subnets: []string{
				"subnet",
			},
			Tags: map[string]string{},
		},
	},
}

func TestFargateProfileGetFunc(t *testing.T) {
	item, err := fargateProfileGetFunc(context.Background(), FargateTestClient, "foo", &eks.DescribeFargateProfileInput{})

	if err != nil {
		t.Error(err)
	}

	if err = item.Validate(); err != nil {
		t.Error(err)
	}

	// It doesn't really make sense to test anything other than the linked items
	// since the attributes are converted automatically
	tests := sources.QueryTests{
		{
			ExpectedType:   "iam-role",
			ExpectedMethod: sdp.QueryMethod_SEARCH,
			ExpectedQuery:  "arn:partition:service::account-id:resource-type/resource-id",
			ExpectedScope:  "account-id",
		},
		{
			ExpectedType:   "ec2-subnet",
			ExpectedMethod: sdp.QueryMethod_GET,
			ExpectedQuery:  "subnet",
			ExpectedScope:  "foo",
		},
	}

	tests.Execute(t, item)
}

func TestNewFargateProfileSource(t *testing.T) {
	config, account, region := sources.GetAutoConfig(t)

	source := NewFargateProfileSource(config, account, region)

	test := sources.E2ETest{
		Source:            source,
		Timeout:           10 * time.Second,
		SkipNotFoundCheck: true,
	}

	test.Run(t)
}
