package system

import (
	"testing"
	"time"
)

func TestWithNetworkRateUsesCounterDeltaAndElapsedTime(t *testing.T) {
	result := withNetworkRate(NetInfo{Rx: 1000, Tx: 500}, NetInfo{Rx: 7000, Tx: 2500}, 5*time.Second)
	if result.RxBytesPerSecond == nil || *result.RxBytesPerSecond != 1200 {
		t.Fatalf("rx rate=%v", result.RxBytesPerSecond)
	}
	if result.TxBytesPerSecond == nil || *result.TxBytesPerSecond != 400 || result.RateWindowMillis != 5000 {
		t.Fatalf("result=%+v", result)
	}
}

func TestWithNetworkRateRejectsCounterReset(t *testing.T) {
	result := withNetworkRate(NetInfo{Rx: 1000, Tx: 500}, NetInfo{Rx: 10, Tx: 5}, 5*time.Second)
	if result.RxBytesPerSecond != nil || result.TxBytesPerSecond != nil || result.RateWindowMillis != 0 {
		t.Fatalf("counter reset produced a rate: %+v", result)
	}
}
