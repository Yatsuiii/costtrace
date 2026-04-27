package aws

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

type CEClient struct {
	ce *costexplorer.Client
}

func NewCEClient(cfg aws.Config) *CEClient {
	return &CEClient{ce: costexplorer.NewFromConfig(cfg)}
}

func (c *CEClient) FetchCosts(ctx context.Context, start, end time.Time) ([]storage.CostRow, error) {
	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String("USAGE_TYPE")},
		},
	}

	var rows []storage.CostRow
	for {
		page, err := c.ce.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("cost explorer: %w", err)
		}
		for _, result := range page.ResultsByTime {
			date := aws.ToString(result.TimePeriod.Start)
			for _, group := range result.Groups {
				if len(group.Keys) < 2 {
					continue
				}
				metric := group.Metrics["UnblendedCost"]
				amt, err := strconv.ParseFloat(aws.ToString(metric.Amount), 64)
				if err != nil {
					continue
				}
				rows = append(rows, storage.CostRow{
					Date:      date,
					Service:   group.Keys[0],
					UsageType: group.Keys[1],
					Amount:    amt,
					Currency:  aws.ToString(metric.Unit),
				})
			}
		}
		if page.NextPageToken == nil {
			break
		}
		input.NextPageToken = page.NextPageToken
	}
	return rows, nil
}
