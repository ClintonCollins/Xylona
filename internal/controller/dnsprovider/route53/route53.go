// Package route53 implements DNS record management through Amazon Route 53.
package route53

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
)

const operationTimeout = 15 * time.Second

type route53API interface {
	ListHostedZones(context.Context, *awsroute53.ListHostedZonesInput, ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesOutput, error)
	GetHostedZone(context.Context, *awsroute53.GetHostedZoneInput, ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error)
	ListResourceRecordSets(context.Context, *awsroute53.ListResourceRecordSetsInput, ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(context.Context, *awsroute53.ChangeResourceRecordSetsInput, ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error)
}

// Client manages records through an Amazon Route 53 client.
type Client struct {
	api route53API
}

// New returns a client using the AWS runtime credential chain.
func New(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	configuration, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, dnsprovider.ErrUnavailable
	}
	return &Client{api: awsroute53.NewFromConfig(configuration)}, nil
}

// NewStatic returns a client using explicit access-key credentials.
func NewStatic(ctx context.Context, accessKeyID string, secretAccessKey string) (*Client, error) {
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, dnsprovider.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	provider := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")
	configuration, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"), config.WithCredentialsProvider(provider))
	if err != nil {
		return nil, dnsprovider.ErrUnavailable
	}
	return &Client{api: awsroute53.NewFromConfig(configuration)}, nil
}

// ListZones lists hosted zones visible to the credentials.
func (client *Client) ListZones(ctx context.Context) ([]dnsprovider.Zone, error) {
	if client == nil || client.api == nil {
		return nil, dnsprovider.ErrUnauthorized
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	var zones []dnsprovider.Zone
	var marker *string
	for {
		output, err := client.api.ListHostedZones(ctx, &awsroute53.ListHostedZonesInput{Marker: marker})
		if err != nil {
			return nil, sanitizeError(err)
		}
		if output == nil {
			return nil, dnsprovider.ErrUnavailable
		}
		for _, hostedZone := range output.HostedZones {
			if hostedZone.Id == nil || hostedZone.Name == nil || *hostedZone.Id == "" || *hostedZone.Name == "" {
				return nil, dnsprovider.ErrUnavailable
			}
			zones = append(zones, dnsprovider.Zone{ID: normalizeZoneID(*hostedZone.Id), Name: normalizeName(*hostedZone.Name)})
		}
		if !output.IsTruncated {
			return zones, nil
		}
		if output.NextMarker == nil || *output.NextMarker == "" {
			return nil, dnsprovider.ErrUnavailable
		}
		marker = output.NextMarker
	}
}

// TestZone verifies access to the exact hosted zone.
func (client *Client) TestZone(ctx context.Context, zone dnsprovider.Zone) error {
	if client == nil || client.api == nil {
		return dnsprovider.ErrUnauthorized
	}
	if zone.ID == "" || zone.Name == "" {
		return dnsprovider.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	output, err := client.api.GetHostedZone(ctx, &awsroute53.GetHostedZoneInput{Id: aws.String(zone.ID)})
	if err != nil {
		return sanitizeError(err)
	}
	if output == nil || output.HostedZone == nil || output.HostedZone.Id == nil || output.HostedZone.Name == nil {
		return dnsprovider.ErrUnavailable
	}
	if normalizeZoneID(*output.HostedZone.Id) != normalizeZoneID(zone.ID) || normalizeName(*output.HostedZone.Name) != normalizeName(zone.Name) {
		return dnsprovider.ErrNotFound
	}
	return nil
}

// ReadRecord reads one supported simple record.
func (client *Client) ReadRecord(ctx context.Context, zone dnsprovider.Zone, key dnsprovider.RecordKey) (dnsprovider.Record, bool, error) {
	if client == nil || client.api == nil {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnauthorized
	}
	if zone.ID == "" || !validRecordType(key.Type) || normalizeName(key.Name) == "." {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	recordType := toRRType(key.Type)
	output, err := client.api.ListResourceRecordSets(ctx, &awsroute53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zone.ID),
		MaxItems:        aws.Int32(2),
		StartRecordName: aws.String(normalizeName(key.Name)),
		StartRecordType: recordType,
	})
	if err != nil {
		return dnsprovider.Record{}, false, sanitizeError(err)
	}
	if output == nil {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnavailable
	}

	wantedName := normalizeName(key.Name)
	var matched []types.ResourceRecordSet
	for _, recordSet := range output.ResourceRecordSets {
		if recordSet.Name == nil || normalizeName(*recordSet.Name) != wantedName || recordSet.Type != recordType {
			continue
		}
		matched = append(matched, recordSet)
	}
	if len(matched) == 0 {
		return dnsprovider.Record{}, false, nil
	}
	if len(matched) != 1 {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnsupported
	}

	record, err := supportedRecord(matched[0])
	if err != nil {
		return dnsprovider.Record{}, false, err
	}
	return record, true, nil
}

// CreateRecord creates one simple record.
func (client *Client) CreateRecord(ctx context.Context, zone dnsprovider.Zone, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	return client.writeRecord(ctx, zone, types.ChangeActionCreate, change)
}

// UpdateRecord replaces one simple record.
func (client *Client) UpdateRecord(ctx context.Context, zone dnsprovider.Zone, _ dnsprovider.Record, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	return client.writeRecord(ctx, zone, types.ChangeActionUpsert, change)
}

func (client *Client) writeRecord(ctx context.Context, zone dnsprovider.Zone, action types.ChangeAction, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	if client == nil || client.api == nil {
		return dnsprovider.Record{}, dnsprovider.ErrUnauthorized
	}
	if zone.ID == "" || !validRecordType(change.Type) || normalizeName(change.Name) == "." || change.Value == "" || change.TTL < 1 {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	providerRecord := types.ResourceRecordSet{
		Name: aws.String(normalizeName(change.Name)),
		Type: toRRType(change.Type),
		TTL:  aws.Int64(change.TTL),
		ResourceRecords: []types.ResourceRecord{
			{Value: aws.String(change.Value)},
		},
	}
	_, err := client.api.ChangeResourceRecordSets(ctx, &awsroute53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zone.ID),
		ChangeBatch: &types.ChangeBatch{Changes: []types.Change{{
			Action:            action,
			ResourceRecordSet: &providerRecord,
		}}},
	})
	if err != nil {
		return dnsprovider.Record{}, sanitizeError(err)
	}
	return dnsprovider.Record{Name: normalizeName(change.Name), Type: change.Type, Value: change.Value, TTL: change.TTL}, nil
}

func supportedRecord(recordSet types.ResourceRecordSet) (dnsprovider.Record, error) {
	if recordSet.Name == nil || recordSet.TTL == nil || *recordSet.TTL < 0 || len(recordSet.ResourceRecords) != 1 || recordSet.ResourceRecords[0].Value == nil {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	if recordSet.AliasTarget != nil || recordSet.CidrRoutingConfig != nil || recordSet.Failover != "" || recordSet.GeoLocation != nil || recordSet.GeoProximityLocation != nil || recordSet.HealthCheckId != nil || recordSet.MultiValueAnswer != nil || recordSet.Region != "" || recordSet.SetIdentifier != nil || recordSet.TrafficPolicyInstanceId != nil || recordSet.Weight != nil {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	recordType := fromRRType(recordSet.Type)
	if !validRecordType(recordType) || *recordSet.ResourceRecords[0].Value == "" {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	return dnsprovider.Record{
		Name:  normalizeName(*recordSet.Name),
		Type:  recordType,
		Value: *recordSet.ResourceRecords[0].Value,
		TTL:   *recordSet.TTL,
	}, nil
}

func sanitizeError(err error) error {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return dnsprovider.ErrUnavailable
	}

	switch apiError.ErrorCode() {
	case "InvalidClientTokenId", "InvalidSignatureException", "SignatureDoesNotMatch", "UnrecognizedClientException":
		return dnsprovider.ErrUnauthorized
	case "AccessDenied", "AccessDeniedException":
		return dnsprovider.ErrForbidden
	case "NoSuchHostedZone":
		return dnsprovider.ErrNotFound
	case "HostedZoneNotEmpty", "InvalidChangeBatch", "PriorRequestNotComplete":
		return dnsprovider.ErrConflict
	default:
		return dnsprovider.ErrUnavailable
	}
}

func normalizeZoneID(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), "/hostedzone/")
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".")) + "."
}

func validRecordType(recordType dnsprovider.RecordType) bool {
	return recordType == dnsprovider.RecordTypeA || recordType == dnsprovider.RecordTypeAAAA
}

func toRRType(recordType dnsprovider.RecordType) types.RRType {
	if recordType == dnsprovider.RecordTypeAAAA {
		return types.RRTypeAaaa
	}
	return types.RRTypeA
}

func fromRRType(recordType types.RRType) dnsprovider.RecordType {
	if recordType == types.RRTypeAaaa {
		return dnsprovider.RecordTypeAAAA
	}
	if recordType == types.RRTypeA {
		return dnsprovider.RecordTypeA
	}
	return ""
}

var _ dnsprovider.Provider = (*Client)(nil)
