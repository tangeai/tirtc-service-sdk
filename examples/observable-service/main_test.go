package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tangeai/tirtc-service-sdk/v2/pkg/whipecho"
)

func TestWriteMetrics(t *testing.T) {
	response := httptest.NewRecorder()
	writeMetrics(response, whipecho.Stats{
		ActiveSessions: 2,
		AcceptedTotal:  3,
		ClosedTotal: map[whipecho.CloseReason]uint64{
			whipecho.CloseReasonDisconnected: 1,
		},
	}, &observedMetrics{})
	body := response.Body.String()
	for _, expected := range []string{
		"whip_echo_active_sessions 2",
		"whip_echo_accepted_total 3",
		`whip_echo_closed_total{reason="disconnected"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, body)
		}
	}
}
