package automation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnoseJobs_FindsInvalidScheduleStuckRunningAndDeliveryProblems(t *testing.T) {
	now := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	findings := DiagnoseJobs([]Job{
		{
			ID:      testServiceJobA,
			Enabled: true,
			Schedule: Schedule{
				Kind: ScheduleCron,
			},
			Delivery: Delivery{Mode: DeliveryWebhook},
			State: JobState{
				RunningAt: now.Add(-2 * time.Hour),
			},
		},
		{
			ID:       testServiceJobB,
			Enabled:  true,
			Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
			Delivery: Delivery{Mode: DeliveryGateway, Channel: "slack"},
		},
		{
			ID:       testServiceJobC,
			Enabled:  true,
			Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
			Delivery: Delivery{Mode: DeliveryMode("unsupported")},
		},
	}, DiagnosticOptions{Now: now, StaleRunningAfter: time.Hour})

	require.Equal(t, []string{
		"invalid_schedule",
		"stuck_running",
		"delivery_webhook_url_missing",
		"delivery_target_incomplete",
		"delivery_mode_unsupported",
	}, diagnosticFindingCodes(findings))
}

func TestDiagnoseJobs_SkipsHealthyJobsAndUsesOriginMetadata(t *testing.T) {
	now := time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC)
	findings := DiagnoseJobs([]Job{
		{
			ID:       testServiceJobA,
			Enabled:  true,
			Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
			Delivery: Delivery{Mode: DeliveryOrigin},
			Payload: Payload{Metadata: map[string]string{
				metadataOriginChannel: "slack",
				metadataOriginTarget:  "channel",
			}},
		},
		{
			ID:       testServiceJobB,
			Enabled:  false,
			Schedule: Schedule{Kind: ScheduleCron},
			Delivery: Delivery{Mode: DeliveryLocal},
			State:    JobState{RunningAt: now.Add(-30 * time.Minute)},
		},
	}, DiagnosticOptions{Now: now, StaleRunningAfter: time.Hour})

	require.Empty(t, findings)
}

func TestDiagnoseDelivery_FindsOriginWithoutCapturedTarget(t *testing.T) {
	findings := DiagnoseDelivery(Job{ID: testServiceJobA, Delivery: Delivery{Mode: DeliveryOrigin}})

	require.Len(t, findings, 1)
	require.Equal(t, "delivery_origin_missing", findings[0].Code)
}

func TestInspectRunHistory_SummarizesLastRunFailuresAndTraceSession(t *testing.T) {
	runs := []Run{
		{
			ID:             testServiceRunA,
			JobID:          testServiceJobA,
			Status:         RunStatusError,
			Error:          "failed",
			SessionID:      testAutomationExecutionSessionID,
			DeliveryStatus: DeliveryStatusNotDelivered,
			DeliveryError:  "delivery failed",
		},
		{
			ID:     testServiceRunB,
			JobID:  testServiceJobA,
			Status: RunStatusError,
			Error:  "older failed",
		},
	}

	inspection := InspectRunHistory(Job{ID: testServiceJobA}, runs, 1)

	require.Equal(t, testServiceRunA, inspection.LastRun.ID)
	require.Equal(t, DeliveryStatusNotDelivered, inspection.DeliveryStatus)
	require.Equal(t, "delivery failed", inspection.DeliveryError)
	require.Equal(t, testAutomationExecutionSessionID, inspection.SessionID)
	require.Len(t, inspection.RecentFailures, 1)
	require.Equal(t, testServiceRunA, inspection.RecentFailures[0].ID)
}

func diagnosticFindingCodes(findings []DiagnosticFinding) []string {
	codes := make([]string, 0, len(findings))
	for _, finding := range findings {
		codes = append(codes, finding.Code)
	}

	return codes
}
