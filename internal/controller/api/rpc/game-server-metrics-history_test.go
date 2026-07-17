package rpc

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestValidateGameServerMetricsHistoryRequest(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		request       *xylona.GetGameServerMetricsHistoryRequest
		wantMaxPoints int
		wantError     bool
	}{
		{
			name: "valid request uses default point limit",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(time.Hour)),
			},
			wantMaxPoints: defaultGameServerMetricsMaxPoints,
		},
		{
			name: "valid explicit maximum",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(maximumGameServerMetricsRange)),
				MaxPoints:    maximumGameServerMetricsMaxPoints,
			},
			wantMaxPoints: maximumGameServerMetricsMaxPoints,
		},
		{
			name: "range beyond previous ninety day limit",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(91 * 24 * time.Hour)),
			},
			wantMaxPoints: defaultGameServerMetricsMaxPoints,
		},
		{
			name:      "nil request",
			request:   nil,
			wantError: true,
		},
		{
			name: "missing server ID",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				Since: timestamppb.New(base),
				Until: timestamppb.New(base.Add(time.Hour)),
			},
			wantError: true,
		},
		{
			name: "missing timestamp",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Until:        timestamppb.New(base.Add(time.Hour)),
			},
			wantError: true,
		},
		{
			name: "invalid timestamp",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        &timestamppb.Timestamp{Seconds: 253402300800},
				Until:        timestamppb.New(base.Add(time.Hour)),
			},
			wantError: true,
		},
		{
			name: "equal timestamps",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base),
			},
			wantError: true,
		},
		{
			name: "range exceeds retention window",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(maximumGameServerMetricsRange + time.Second)),
			},
			wantError: true,
		},
		{
			name: "negative point limit",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(time.Hour)),
				MaxPoints:    -1,
			},
			wantError: true,
		},
		{
			name: "point limit exceeds maximum",
			request: &xylona.GetGameServerMetricsHistoryRequest{
				GameServerId: "server-1",
				Since:        timestamppb.New(base),
				Until:        timestamppb.New(base.Add(time.Hour)),
				MaxPoints:    maximumGameServerMetricsMaxPoints + 1,
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, maxPoints, errValidate := validateGameServerMetricsHistoryRequest(test.request)
			if test.wantError {
				if errValidate == nil {
					t.Fatal("expected validation error")
				}
				return
			}
			if errValidate != nil {
				t.Fatalf("validate request: %v", errValidate)
			}
			if maxPoints != test.wantMaxPoints {
				t.Fatalf("max points = %d, want %d", maxPoints, test.wantMaxPoints)
			}
		})
	}
}

func TestBuildGameServerMetricsHistory(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	rows := []*db.GameServerMetricsRow{
		availableMetricsRow(base, 60, 10, 100, 20, 2),
		{
			RecordedAt:           base.Add(time.Hour),
			GranularitySeconds:   60,
			SampleCount:          1,
			AvailableSampleCount: 0,
			CollectionStatus:     "node_unavailable",
			MemoryBytes:          9999,
			CPUPercent:           9999,
		},
		availableMetricsRow(base.Add(2*time.Hour), 3600, 20, 200, 40, 4),
		availableMetricsRow(base.Add(3*time.Hour), 3600, 30, 300, 60, 6),
	}
	rows[0].CPUPercentMin = sql.NullFloat64{Float64: 5, Valid: true}
	rows[0].CPUPercentMax = sql.NullFloat64{Float64: 95, Valid: true}
	rows[0].MemoryBytesMin = sql.NullInt64{Int64: 90, Valid: true}
	rows[0].MemoryBytesMax = sql.NullInt64{Int64: 150, Valid: true}
	rows[3].SampleCount = 3
	rows[3].AvailableSampleCount = 3

	series := buildGameServerMetricsHistory(rows, base, base.Add(4*time.Hour), 2)
	if series.resolution != xylona.GameServerMetricsResolution_GAME_SERVER_METRICS_RESOLUTION_DOWNSAMPLED {
		t.Fatalf("resolution = %v, want downsampled", series.resolution)
	}
	if !series.hasMixedResolution {
		t.Fatal("expected mixed source resolution to be reported")
	}
	if series.sampleIntervalSeconds != 7200 {
		t.Fatalf("sample interval = %d, want 7200", series.sampleIntervalSeconds)
	}
	if len(series.points) != 2 {
		t.Fatalf("point count = %d, want 2", len(series.points))
	}

	first := series.points[0]
	if first.GetAvailabilityRatio() != 0.5 {
		t.Fatalf("availability ratio = %v, want 0.5", first.GetAvailabilityRatio())
	}
	if first.GetMemoryRssBytes() != 100 {
		t.Fatalf("memory average = %d, want unavailable sample excluded", first.GetMemoryRssBytes())
	}
	if !first.GetCpuValid() || first.GetCpuPercent() != 10 {
		t.Fatalf("CPU = (%v, %v), want valid 10", first.GetCpuValid(), first.GetCpuPercent())
	}
	if first.CpuPercentMax == nil || first.GetCpuPercentMax() != 95 {
		t.Fatalf("CPU maximum = %v, want 95", first.CpuPercentMax)
	}
	if first.MemoryRssBytesMax == nil || first.GetMemoryRssBytesMax() != 150 {
		t.Fatalf("memory maximum = %v, want 150", first.MemoryRssBytesMax)
	}
	if first.GetCollectionStatus() != xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_AVAILABLE {
		t.Fatalf("collection status = %v, want available with partial coverage", first.GetCollectionStatus())
	}
	second := series.points[1]
	if second.GetMemoryRssBytes() != 275 || second.GetCpuPercent() != 27.5 {
		t.Fatalf("weighted values = (memory %d, CPU %v), want (275, 27.5)", second.GetMemoryRssBytes(), second.GetCpuPercent())
	}
}

func TestBuildGameServerMetricsHistoryPreservesMetricSpecificWeights(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	mostlyInvalid := availableMetricsRow(base, 3600, 60, 100, 10, 12)
	mostlyInvalid.SampleCount = 60
	mostlyInvalid.AvailableSampleCount = 60
	mostlyInvalid.CPUValidSampleCount = 1
	mostlyInvalid.QuerySuccessfulSampleCount = 1
	mostlyInvalid.QueryDurationValidSampleCount = 1
	mostlyInvalid.ServerFPSValidSampleCount = 1
	mostlyInvalid.ServerFrameTimeValidSampleCount = 1
	mostlyInvalid.QueryDurationMS = sql.NullFloat64{Float64: 600, Valid: true}
	mostlyInvalid.ServerFPS = sql.NullFloat64{Float64: 60, Valid: true}
	mostlyInvalid.ServerFrameTimeMS = sql.NullFloat64{Float64: 10, Valid: true}
	mostlyInvalid.VolumeValid = true
	mostlyInvalid.VolumeValidSampleCount = 1
	mostlyInvalid.IOValidSampleCount = 1
	mostlyInvalid.ConnectionValidSampleCount = 1
	mostlyInvalid.DiskUsageBytes = 600

	fullyValid := availableMetricsRow(base.Add(time.Hour), 3600, 20, 200, 20, 2)
	fullyValid.SampleCount = 60
	fullyValid.AvailableSampleCount = 60
	fullyValid.CPUValidSampleCount = 60
	fullyValid.QuerySuccessfulSampleCount = 60
	fullyValid.QueryDurationValidSampleCount = 60
	fullyValid.ServerFPSValidSampleCount = 60
	fullyValid.ServerFrameTimeValidSampleCount = 60
	fullyValid.QueryDurationMS = sql.NullFloat64{Float64: 200, Valid: true}
	fullyValid.ServerFPS = sql.NullFloat64{Float64: 20, Valid: true}
	fullyValid.ServerFrameTimeMS = sql.NullFloat64{Float64: 30, Valid: true}
	fullyValid.VolumeValid = true
	fullyValid.VolumeValidSampleCount = 60
	fullyValid.IOValidSampleCount = 60
	fullyValid.ConnectionValidSampleCount = 60
	fullyValid.DiskUsageBytes = 200

	series := buildGameServerMetricsHistory(
		[]*db.GameServerMetricsRow{mostlyInvalid, fullyValid},
		base,
		base.Add(2*time.Hour),
		1,
	)
	if len(series.points) != 1 {
		t.Fatalf("point count = %d, want 1", len(series.points))
	}

	point := series.points[0]
	wantCPU := float64(60+20*60) / 61
	if math.Abs(point.GetCpuPercent()-wantCPU) > 0.000001 {
		t.Fatalf("CPU average = %v, want %v", point.GetCpuPercent(), wantCPU)
	}
	if point.GetPlayerCount() != 2 {
		t.Fatalf("player average = %d, want 2", point.GetPlayerCount())
	}
	if point.GetDiskUsageBytes() != 207 {
		t.Fatalf("disk average = %d, want 207", point.GetDiskUsageBytes())
	}
	wantQueryDuration := float64(600+200*60) / 61
	if math.Abs(point.GetQueryDurationMs()-wantQueryDuration) > 0.000001 {
		t.Fatalf("query duration average = %v, want %v", point.GetQueryDurationMs(), wantQueryDuration)
	}
	wantServerFPS := float64(60+20*60) / 61
	if math.Abs(point.GetServerFps()-wantServerFPS) > 0.000001 {
		t.Fatalf("server FPS average = %v, want %v", point.GetServerFps(), wantServerFPS)
	}
	wantFrameTime := float64(10+30*60) / 61
	if math.Abs(point.GetServerFrameTimeMs()-wantFrameTime) > 0.000001 {
		t.Fatalf("server frame time average = %v, want %v", point.GetServerFrameTimeMs(), wantFrameTime)
	}
	if point.GetCpuValidSampleCount() != 61 || point.GetQuerySuccessfulSampleCount() != 61 {
		t.Fatalf(
			"validity weights = (%d CPU, %d query), want (61, 61)",
			point.GetCpuValidSampleCount(),
			point.GetQuerySuccessfulSampleCount(),
		)
	}
	if point.GetVolumeValidSampleCount() != 61 {
		t.Fatalf("volume validity weight = %d, want 61", point.GetVolumeValidSampleCount())
	}
	if point.GetQueryDurationValidSampleCount() != 61 || point.GetServerFpsValidSampleCount() != 61 || point.GetServerFrameTimeValidSampleCount() != 61 {
		t.Fatalf(
			"query performance weights = (%d duration, %d FPS, %d frame time), want (61, 61, 61)",
			point.GetQueryDurationValidSampleCount(),
			point.GetServerFpsValidSampleCount(),
			point.GetServerFrameTimeValidSampleCount(),
		)
	}
	if point.GetIoValidSampleCount() != 61 || point.GetConnectionValidSampleCount() != 61 {
		t.Fatalf(
			"metric-specific weights = (%d IO, %d connection), want (61, 61)",
			point.GetIoValidSampleCount(),
			point.GetConnectionValidSampleCount(),
		)
	}
}

func TestMapGameServerMetricsHistoryPoint(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name              string
		row               *db.GameServerMetricsRow
		wantStatus        xylona.GameServerMetricsCollectionStatus
		wantMemoryTotal   *int64
		wantPlayerValid   bool
		wantOptionalPeaks bool
	}{
		{
			name: "explicit available values and nullable dimensions",
			row: &db.GameServerMetricsRow{
				RecordedAt:            base,
				GranularitySeconds:    60,
				SampleCount:           2,
				AvailableSampleCount:  2,
				AvailabilityRatio:     1,
				CollectionStatus:      "available",
				CPUValid:              true,
				CPUPercent:            25,
				CPUPercentMin:         sql.NullFloat64{Float64: 10, Valid: true},
				CPUPercentMax:         sql.NullFloat64{Float64: 70, Valid: true},
				MemoryBytes:           512,
				MemoryBytesMin:        sql.NullInt64{Int64: 400, Valid: true},
				MemoryBytesMax:        sql.NullInt64{Int64: 700, Valid: true},
				NodeMemoryTotalBytes:  sql.NullInt64{Int64: 4096, Valid: true},
				ConfiguredMemoryBytes: sql.NullInt64{Int64: 1024, Valid: true},
				QuerySupported:        sql.NullBool{Bool: true, Valid: true},
				QuerySuccess:          sql.NullBool{Bool: true, Valid: true},
				PlayerCount:           4,
			},
			wantStatus:        xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_AVAILABLE,
			wantMemoryTotal:   int64Pointer(4096),
			wantPlayerValid:   true,
			wantOptionalPeaks: true,
		},
		{
			name: "unknown remains unavailable rather than valid zero",
			row: &db.GameServerMetricsRow{
				RecordedAt:         base,
				GranularitySeconds: 60,
				SampleCount:        1,
				CollectionStatus:   "node_unavailable",
				QuerySupported:     sql.NullBool{Bool: true, Valid: true},
				QuerySuccess:       sql.NullBool{Bool: false, Valid: true},
			},
			wantStatus: xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_NODE_UNAVAILABLE,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			point := mapGameServerMetricsHistoryPoint(test.row)
			if point.GetCollectionStatus() != test.wantStatus {
				t.Fatalf("collection status = %v, want %v", point.GetCollectionStatus(), test.wantStatus)
			}
			if point.GetPlayerCountValid() != test.wantPlayerValid {
				t.Fatalf("player valid = %v, want %v", point.GetPlayerCountValid(), test.wantPlayerValid)
			}
			if test.wantMemoryTotal == nil {
				if point.NodeMemoryTotalBytes != nil {
					t.Fatalf("node memory total = %v, want nil", point.NodeMemoryTotalBytes)
				}
			} else if point.NodeMemoryTotalBytes == nil || point.GetNodeMemoryTotalBytes() != *test.wantMemoryTotal {
				t.Fatalf("node memory total = %v, want %v", point.NodeMemoryTotalBytes, *test.wantMemoryTotal)
			}
			if test.wantOptionalPeaks {
				if point.CpuPercentMax == nil || point.MemoryRssBytesMax == nil {
					t.Fatal("expected explicit peak fields")
				}
			} else if point.CpuPercentMax != nil || point.MemoryRssBytesMax != nil {
				t.Fatal("unknown metrics must not gain zero-valued peaks")
			}
		})
	}
}

func availableMetricsRow(recordedAt time.Time, granularity int64, cpu float64, memory int64, memoryPercent float64, players int64) *db.GameServerMetricsRow {
	return &db.GameServerMetricsRow{
		RecordedAt:           recordedAt,
		GranularitySeconds:   granularity,
		SampleCount:          1,
		AvailableSampleCount: 1,
		AvailabilityRatio:    1,
		CollectionStatus:     "available",
		CPUValid:             true,
		CPUPercent:           cpu,
		MemoryBytes:          memory,
		MemoryPercent:        memoryPercent,
		QuerySupported:       sql.NullBool{Bool: true, Valid: true},
		QuerySuccess:         sql.NullBool{Bool: true, Valid: true},
		PlayerCount:          players,
	}
}

func int64Pointer(value int64) *int64 {
	return new(value)
}
