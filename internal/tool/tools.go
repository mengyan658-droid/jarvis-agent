package tool

import (
	"context"
	"time"

	"jarvis-agent/internal/client"
)

const (
	QueryHostsToolName   = "query_hosts"
	GetHostToolName      = "get_host"
	QueryMetricsToolName = "query_metrics"
	QueryAlarmsToolName  = "query_alarms"
	QueryChangesToolName = "query_changes"
	QueryCMDBToolName    = "query_cmdb"
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
	HostID string
	Since  time.Time
}

type QueryChangesTool struct{ Client client.ChangeClient }

func (t QueryChangesTool) Name() string { return QueryChangesToolName }
func (t QueryChangesTool) Execute(ctx context.Context, input any) (any, error) {
	in := input.(QueryChangesInput)
	return t.Client.QueryRecentChanges(ctx, in.HostID, in.Since)
}

type QueryCMDBInput struct{ HostID string }

type QueryCMDBTool struct{ Client client.CMDBClient }

func (t QueryCMDBTool) Name() string { return QueryCMDBToolName }
func (t QueryCMDBTool) Execute(ctx context.Context, input any) (any, error) {
	return t.Client.QueryHostMetadata(ctx, input.(QueryCMDBInput).HostID)
}
