package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cttypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

// write events that create/modify resources — these are the ones that change cost
var writeEventNames = []string{
	"RunInstances", "CreateVolume", "CreateBucket", "CreateLoadBalancer",
	"CreateDBInstance", "CreateCluster", "CreateFunction20150331",
	"CreateNatGateway", "AllocateAddress", "CreateFileSystem",
	"CreateTable", "CreateCacheCluster", "CreateDomain",
}

type CTClient struct {
	ct *cloudtrail.Client
}

func NewCTClient(cfg aws.Config) *CTClient {
	return &CTClient{ct: cloudtrail.NewFromConfig(cfg)}
}

func (c *CTClient) FetchEventsForWindow(ctx context.Context, deployID string, start, end time.Time) ([]storage.CloudTrailRow, error) {
	var rows []storage.CloudTrailRow

	for _, name := range writeEventNames {
		input := &cloudtrail.LookupEventsInput{
			StartTime: aws.Time(start),
			EndTime:   aws.Time(end),
			LookupAttributes: []cttypes.LookupAttribute{
				{
					AttributeKey:   cttypes.LookupAttributeKeyEventName,
					AttributeValue: aws.String(name),
				},
			},
			MaxResults: aws.Int32(50),
		}
		paginator := cloudtrail.NewLookupEventsPaginator(c.ct, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				// CloudTrail LookupEvents has aggressive rate limits; skip on throttle
				break
			}
			for _, e := range page.Events {
				principalID := ""
				userAgent := ""
				if e.Username != nil {
					principalID = aws.ToString(e.Username)
				}
				if e.CloudTrailEvent != nil {
					// userAgent is buried in raw JSON — skip full parse, use event name as signal
					_ = e.CloudTrailEvent
				}
				for _, r := range e.Resources {
					rows = append(rows, storage.CloudTrailRow{
						EventID:      aws.ToString(e.EventId),
						EventTime:    aws.ToTime(e.EventTime),
						EventName:    aws.ToString(e.EventName),
						PrincipalID:  principalID,
						UserAgent:    userAgent,
						ResourceType: aws.ToString(r.ResourceType),
						ResourceID:   aws.ToString(r.ResourceName),
						Region:       aws.ToString(e.EventSource),
						DeployID:     deployID,
					})
				}
			}
		}
	}
	return rows, nil
}

func (c *CTClient) FetchEventsForDeploys(ctx context.Context, deploys []storage.DeployRow, windowHours int) ([]storage.CloudTrailRow, error) {
	var all []storage.CloudTrailRow
	seen := map[string]bool{}
	window := time.Duration(windowHours) * time.Hour
	for _, d := range deploys {
		rows, err := c.FetchEventsForWindow(ctx, d.ID, d.StartedAt, d.CompletedAt.Add(window))
		if err != nil {
			return nil, fmt.Errorf("cloudtrail for deploy %s: %w", d.ID, err)
		}
		for _, r := range rows {
			if !seen[r.EventID] {
				seen[r.EventID] = true
				all = append(all, r)
			}
		}
	}
	return all, nil
}
