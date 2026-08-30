package route53

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
)

func TestClient(t *testing.T) {
	t.Run("rejects incomplete static credentials", func(t *testing.T) {
		_, err := NewStatic(t.Context(), "access-key", "")
		if !errors.Is(err, dnsprovider.ErrUnauthorized) {
			t.Fatalf("NewStatic() error = %v", err)
		}
	})

	t.Run("lists and tests zones", func(t *testing.T) {
		fake := &fakeAPI{
			listZonesOutput: &awsroute53.ListHostedZonesOutput{HostedZones: []types.HostedZone{{Id: aws.String("/hostedzone/Z1"), Name: aws.String("Example.COM.")}}},
			getZoneOutput:   &awsroute53.GetHostedZoneOutput{HostedZone: &types.HostedZone{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.com.")}},
		}
		client := &Client{api: fake}

		zones, err := client.ListZones(t.Context())
		if err != nil {
			t.Fatalf("ListZones() error = %v", err)
		}
		if len(zones) != 1 || zones[0].ID != "Z1" || zones[0].Name != "example.com." {
			t.Fatalf("ListZones() = %#v", zones)
		}

		err = client.TestZone(t.Context(), dnsprovider.Zone{ID: "Z1", Name: "EXAMPLE.COM"})
		if err != nil {
			t.Fatalf("TestZone() error = %v", err)
		}
	})

	t.Run("reads one simple record and rejects unsupported shapes", func(t *testing.T) {
		baseRecord := types.ResourceRecordSet{
			Name:            aws.String("Game.Example.com."),
			Type:            types.RRTypeA,
			TTL:             aws.Int64(0),
			ResourceRecords: []types.ResourceRecord{{Value: aws.String("192.0.2.10")}},
		}
		fake := &fakeAPI{listRecordsOutput: &awsroute53.ListResourceRecordSetsOutput{ResourceRecordSets: []types.ResourceRecordSet{baseRecord}}}
		client := &Client{api: fake}
		key := dnsprovider.RecordKey{Name: "game.example.com.", Type: dnsprovider.RecordTypeA}

		record, found, err := client.ReadRecord(t.Context(), dnsprovider.Zone{ID: "Z1"}, key)
		if err != nil || !found {
			t.Fatalf("ReadRecord() = %#v, %v, %v", record, found, err)
		}
		if record.ID != "" || record.Name != key.Name || record.Value != "192.0.2.10" || record.TTL != 0 {
			t.Fatalf("ReadRecord() = %#v", record)
		}

		unsupported := []types.ResourceRecordSet{
			{Name: baseRecord.Name, Type: types.RRTypeA, AliasTarget: &types.AliasTarget{}},
			{Name: baseRecord.Name, Type: types.RRTypeA, TTL: baseRecord.TTL, ResourceRecords: append(baseRecord.ResourceRecords, types.ResourceRecord{Value: aws.String("192.0.2.11")})},
			{Name: baseRecord.Name, Type: types.RRTypeA, TTL: baseRecord.TTL, ResourceRecords: baseRecord.ResourceRecords, SetIdentifier: aws.String("weighted"), Weight: aws.Int64(1)},
			{Name: baseRecord.Name, Type: types.RRTypeA, TTL: baseRecord.TTL, ResourceRecords: baseRecord.ResourceRecords, MultiValueAnswer: aws.Bool(true)},
		}
		for index, recordSet := range unsupported {
			fake.listRecordsOutput.ResourceRecordSets = []types.ResourceRecordSet{recordSet}
			_, _, err = client.ReadRecord(t.Context(), dnsprovider.Zone{ID: "Z1"}, key)
			if !errors.Is(err, dnsprovider.ErrUnsupported) {
				t.Fatalf("unsupported case %d error = %v", index, err)
			}
		}
	})

	t.Run("applies an operation deadline to every provider call", func(t *testing.T) {
		fake := &fakeAPI{
			listZonesOutput:   &awsroute53.ListHostedZonesOutput{},
			getZoneOutput:     &awsroute53.GetHostedZoneOutput{HostedZone: &types.HostedZone{Id: aws.String("Z1"), Name: aws.String("example.com.")}},
			listRecordsOutput: &awsroute53.ListResourceRecordSetsOutput{},
		}
		client := &Client{api: fake}
		ctx := context.WithoutCancel(t.Context())
		zone := dnsprovider.Zone{ID: "Z1", Name: "example.com."}
		key := dnsprovider.RecordKey{Name: "game.example.com.", Type: dnsprovider.RecordTypeA}
		change := dnsprovider.RecordChange{Name: key.Name, Type: key.Type, Value: "192.0.2.10", TTL: 300}

		_, err := client.ListZones(ctx)
		if err != nil {
			t.Fatalf("ListZones() error = %v", err)
		}
		err = client.TestZone(ctx, zone)
		if err != nil {
			t.Fatalf("TestZone() error = %v", err)
		}
		_, _, err = client.ReadRecord(ctx, zone, key)
		if err != nil {
			t.Fatalf("ReadRecord() error = %v", err)
		}
		created, err := client.CreateRecord(ctx, zone, change)
		if err != nil {
			t.Fatalf("CreateRecord() error = %v", err)
		}
		_, err = client.UpdateRecord(ctx, zone, created, change)
		if err != nil {
			t.Fatalf("UpdateRecord() error = %v", err)
		}

		if len(fake.deadlines) != 5 {
			t.Fatalf("provider deadlines = %d, want 5", len(fake.deadlines))
		}
		for index, deadline := range fake.deadlines {
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > operationTimeout {
				t.Fatalf("provider deadline %d remaining = %v, want within %v", index, remaining, operationTimeout)
			}
		}
	})

	t.Run("creates and updates with explicit actions", func(t *testing.T) {
		fake := &fakeAPI{}
		client := &Client{api: fake}
		change := dnsprovider.RecordChange{Name: "game.example.com.", Type: dnsprovider.RecordTypeAAAA, Value: "2001:db8::1", TTL: 300}

		created, err := client.CreateRecord(t.Context(), dnsprovider.Zone{ID: "Z1"}, change)
		if err != nil {
			t.Fatalf("CreateRecord() error = %v", err)
		}
		_, err = client.UpdateRecord(t.Context(), dnsprovider.Zone{ID: "Z1"}, created, change)
		if err != nil {
			t.Fatalf("UpdateRecord() error = %v", err)
		}
		if len(fake.changes) != 2 || fake.changes[0] != types.ChangeActionCreate || fake.changes[1] != types.ChangeActionUpsert {
			t.Fatalf("actions = %v", fake.changes)
		}
	})

	t.Run("sanitizes SDK failures", func(t *testing.T) {
		fake := &fakeAPI{err: &smithy.GenericAPIError{Code: "AccessDenied", Message: "secret SDK detail"}}
		client := &Client{api: fake}
		_, err := client.ListZones(t.Context())
		if !errors.Is(err, dnsprovider.ErrForbidden) {
			t.Fatalf("ListZones() error = %v", err)
		}
		if strings.Contains(err.Error(), "secret SDK detail") {
			t.Fatalf("error leaked SDK detail: %v", err)
		}
	})
}

type fakeAPI struct {
	listZonesOutput   *awsroute53.ListHostedZonesOutput
	getZoneOutput     *awsroute53.GetHostedZoneOutput
	listRecordsOutput *awsroute53.ListResourceRecordSetsOutput
	err               error
	changes           []types.ChangeAction
	deadlines         []time.Time
}

func (fake *fakeAPI) ListHostedZones(ctx context.Context, _ *awsroute53.ListHostedZonesInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesOutput, error) {
	fake.recordDeadline(ctx)
	return fake.listZonesOutput, fake.err
}

func (fake *fakeAPI) GetHostedZone(ctx context.Context, _ *awsroute53.GetHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error) {
	fake.recordDeadline(ctx)
	return fake.getZoneOutput, fake.err
}

func (fake *fakeAPI) ListResourceRecordSets(ctx context.Context, _ *awsroute53.ListResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
	fake.recordDeadline(ctx)
	return fake.listRecordsOutput, fake.err
}

func (fake *fakeAPI) ChangeResourceRecordSets(ctx context.Context, input *awsroute53.ChangeResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
	fake.recordDeadline(ctx)
	if input != nil && input.ChangeBatch != nil && len(input.ChangeBatch.Changes) == 1 {
		fake.changes = append(fake.changes, input.ChangeBatch.Changes[0].Action)
	}
	return &awsroute53.ChangeResourceRecordSetsOutput{}, fake.err
}

func (fake *fakeAPI) recordDeadline(ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if ok {
		fake.deadlines = append(fake.deadlines, deadline)
	}
}
