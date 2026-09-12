package futar

import (
	"testing"
	"time"
)

func TestToTimeSecondsOrMillis(t *testing.T) {
	want := time.Date(2026, 9, 12, 20, 30, 0, 0, time.UTC)
	if got := toTime(want.Unix()); !got.Equal(want) {
		t.Errorf("seconds: %v", got)
	}
	if got := toTime(want.UnixMilli()); !got.Equal(want) {
		t.Errorf("millis: %v", got)
	}
	if !toTime(0).IsZero() {
		t.Error("zero must stay zero")
	}
}

func TestStopTimeAccessors(t *testing.T) {
	st := StopTime{DepartureTime: 100, ArrivalTime: 90, PredictedDepartureTime: 110}
	if p, live := st.Predicted(); !live || p.Unix() != 110 {
		t.Errorf("Predicted = %v %v", p, live)
	}
	if st.Scheduled().Unix() != 100 {
		t.Error("Scheduled should prefer departure")
	}
	st.PredictionScheduled = true
	if _, live := st.Predicted(); live {
		t.Error("predictionScheduled must not count as live")
	}
	only := StopTime{ArrivalTime: 90, PredictedArrivalTime: 95}
	if p, live := only.Predicted(); !live || p.Unix() != 95 {
		t.Error("should fall back to predicted arrival")
	}
	no := false
	if (StopTime{PickupAllowed: &no}).Boardable() || !(StopTime{}).Boardable() {
		t.Error("Boardable")
	}
}
