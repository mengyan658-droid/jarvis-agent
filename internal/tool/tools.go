package tool

import (
	"context"
	"fmt"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/domain"
)

const (
	QueryHostsToolName              = "query_hosts"
	GetHostToolName                 = "get_host"
	QueryMetricsToolName            = "query_metrics"
	QueryAlarmsToolName             = "query_alarms"
	QueryChangesToolName            = "query_changes"
	QueryCMDBToolName               = "query_cmdb"
	QueryErrorRequestCountsToolName = "query_error_request_counts"
)

type QueryHostsTool struct{ Client client.JarvisClient }

func (t QueryHostsTool) Name() string { return QueryHostsToolName }
func (t QueryHostsTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.QueryHosts(ctx, input.(client.HostQuery))
}

type GetHostInput struct{ HostID string }

type GetHostTool struct{ Client client.JarvisClient }

func (t GetHostTool) Name() string { return GetHostToolName }
func (t GetHostTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.GetHost(ctx, input.(GetHostInput).HostID)
}

type QueryMetricsInput struct{ HostID string }

type QueryMetricsTool struct{ Client client.MonitorClient }

func (t QueryMetricsTool) Name() string { return QueryMetricsToolName }
func (t QueryMetricsTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.QueryHostMetrics(ctx, input.(QueryMetricsInput).HostID)
}

type QueryAlarmsInput struct{ HostID string }

type QueryAlarmsTool struct{ Client client.MonitorClient }

func (t QueryAlarmsTool) Name() string { return QueryAlarmsToolName }
func (t QueryAlarmsTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.QueryActiveAlarms(ctx, input.(QueryAlarmsInput).HostID)
}

type QueryChangesInput struct {
	HostID    string
	TimeRange domain.TimeRange
}

type QueryChangesTool struct{ Client client.ChangeClient }

func (t QueryChangesTool) Name() string { return QueryChangesToolName }
func (t QueryChangesTool) Execute(ctx context.Context, input any) (any, error) {
	in := input.(QueryChangesInput)
	if in.HostID == "" {
		return nil, fmt.Errorf("host_id is required")
	}
	if in.TimeRange.Start.IsZero() || in.TimeRange.End.IsZero() {
		return nil, fmt.Errorf("time_range start and end are required")
	}
	if !in.TimeRange.Start.Before(in.TimeRange.End) {
		return nil, fmt.Errorf("time_range start must be before end")
	}
	return t.Client.QueryRecentChanges(ctx, in.HostID, in.TimeRange)
}

type QueryCMDBInput struct{ HostID string }

type QueryCMDBTool struct{ Client client.CMDBClient }

func (t QueryCMDBTool) Name() string { return QueryCMDBToolName }
func (t QueryCMDBTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.QueryHostMetadata(ctx, input.(QueryCMDBInput).HostID)
}

type QueryErrorRequestCountsInput struct {
	TimeRange    domain.TimeRange
	DeviceModels []string
	IDCs         []string
	ErrorCode    string
	Aggregation  domain.TimeAggregation
}

type QueryErrorRequestCountsTool struct{ Client client.RequestCountClient }

func (t QueryErrorRequestCountsTool) Name() string { return QueryErrorRequestCountsToolName }
func (t QueryErrorRequestCountsTool) Execute(ctx context.Context, input any) (any, error) {
	in := input.(QueryErrorRequestCountsInput)
	if in.TimeRange.Start.IsZero() || in.TimeRange.End.IsZero() {
		return nil, fmt.Errorf("time_range start and end are required")
	}
	if !in.TimeRange.Start.Before(in.TimeRange.End) {
		return nil, fmt.Errorf("time_range start must be before end")
	}
	if _, err := in.Aggregation.Duration(); err != nil {
		return nil, err
	}
	return t.Client.QueryErrorRequestCounts(ctx, client.ErrorRequestCountQuery{
		TimeRange:    in.TimeRange,
		DeviceModels: in.DeviceModels,
		IDCs:         in.IDCs,
		ErrorCode:    in.ErrorCode,
		Aggregation:  in.Aggregation,
	})
}
