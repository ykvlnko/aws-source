package networkmanager

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/networkmanager"
	"github.com/aws/aws-sdk-go-v2/service/networkmanager/types"
	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/sdp-go"
	"github.com/stretchr/testify/require"
	"testing"
)

func (t *TestClient) GetSites(ctx context.Context, params *networkmanager.GetSitesInput, optFns ...func(*networkmanager.Options)) (*networkmanager.GetSitesOutput, error) {
	return &networkmanager.GetSitesOutput{
		Sites: []types.Site{
			{
				Tags:            []types.Tag{},
				SiteId:          sources.PtrString("site1"),
				GlobalNetworkId: sources.PtrString("default"),
			},
			{
				Tags:            []types.Tag{},
				SiteId:          sources.PtrString("site2"),
				GlobalNetworkId: sources.PtrString("other"),
			},
		},
	}, nil
}

func TestSiteOutputMapper(t *testing.T) {
	output := networkmanager.GetSitesOutput{
		Sites: []types.Site{
			{
				SiteId:          sources.PtrString("site1"),
				GlobalNetworkId: sources.PtrString("default"),
			},
		},
	}
	scope := "123456789012.eu-west-2"
	items, err := siteOutputMapper(context.Background(), &TestClient{}, scope, &networkmanager.GetSitesInput{}, &output)

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

	// Ensure unique attribute
	require.NotNil(t, item.Attributes)
	uniqueAttr, err := item.Attributes.Get("globalNetworkSiteId")
	require.Nil(t, err)
	require.Equal(t, "default/site1", uniqueAttr.(string))

	tests := sources.QueryTests{
		{
			ExpectedType:   "networkmanager-global-network",
			ExpectedMethod: sdp.QueryMethod_GET,
			ExpectedQuery:  "default",
			ExpectedScope:  scope,
		},
	}

	tests.Execute(t, item)
}
