package ec2

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/overmindtech/aws-source/sources"
)

func TestReservedInstanceInputMapperGet(t *testing.T) {
	input, err := reservedInstanceInputMapperGet("foo", "bar")

	if err != nil {
		t.Error(err)
	}

	if len(input.ReservedInstancesIds) != 1 {
		t.Fatalf("expected 1 Reservedinstance ID, got %v", len(input.ReservedInstancesIds))
	}

	if input.ReservedInstancesIds[0] != "bar" {
		t.Errorf("expected Reservedinstance ID to be bar, got %v", input.ReservedInstancesIds[0])
	}
}

func TestReservedInstanceInputMapperList(t *testing.T) {
	input, err := reservedInstanceInputMapperList("foo")

	if err != nil {
		t.Error(err)
	}

	if len(input.Filters) != 0 || len(input.ReservedInstancesIds) != 0 {
		t.Errorf("non-empty input: %v", input)
	}
}

func TestReservedInstanceOutputMapper(t *testing.T) {
	output := &ec2.DescribeReservedInstancesOutput{
		ReservedInstances: []types.ReservedInstances{
			{
				AvailabilityZone:   sources.PtrString("az"),
				CurrencyCode:       types.CurrencyCodeValuesUsd,
				Duration:           sources.PtrInt64(100),
				End:                sources.PtrTime(time.Now()),
				FixedPrice:         sources.PtrFloat32(1.23),
				InstanceCount:      sources.PtrInt32(1),
				InstanceTenancy:    types.TenancyDedicated,
				InstanceType:       types.InstanceTypeA14xlarge,
				OfferingClass:      types.OfferingClassTypeConvertible,
				OfferingType:       types.OfferingTypeValuesAllUpfront,
				ProductDescription: types.RIProductDescription("foo"),
				RecurringCharges: []types.RecurringCharge{
					{
						Amount:    sources.PtrFloat64(1.111),
						Frequency: types.RecurringChargeFrequencyHourly,
					},
				},
				ReservedInstancesId: sources.PtrString("id"),
				Scope:               types.ScopeAvailabilityZone,
				Start:               sources.PtrTime(time.Now()),
				State:               types.ReservedInstanceStateActive,
				UsagePrice:          sources.PtrFloat32(99.00000001),
			},
		},
	}

	items, err := reservedInstanceOutputMapper(context.Background(), nil, "foo", nil, output)

	if err != nil {
		t.Fatal(err)
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
	tests := sources.QueryTests{}

	tests.Execute(t, item)

}

func TestNewReservedInstanceSource(t *testing.T) {
	config, account, _ := sources.GetAutoConfig(t)

	source := NewReservedInstanceSource(config, account, &TestRateLimit)

	test := sources.E2ETest{
		Source:  source,
		Timeout: 10 * time.Second,
	}

	test.Run(t)
}
