package athq

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(512); got != "512 B" {
		t.Errorf("512: got = %q, want %q", got, "512 B")
	}
	if got := humanBytes(1536); got != "1.50 KiB" {
		t.Errorf("1536: got = %q, want %q", got, "1.50 KiB")
	}
	if got := humanBytes(3 * 1024 * 1024 * 1024); got != "3.00 GiB" {
		t.Errorf("3GiB: got = %q, want %q", got, "3.00 GiB")
	}
}

func TestEstimateCostUsesTheMinimumBilledSize(t *testing.T) {
	// Anything under 10 MB is billed as 10 MB.
	small := estimateCost(1, 5.0)
	tenMB := estimateCost(10*1024*1024, 5.0)
	if small != tenMB {
		t.Errorf("got = %v, want the same as the 10 MB estimate %v", small, tenMB)
	}
	oneTB := estimateCost(bytesPerTB, 5.0)
	if oneTB != 5.0 {
		t.Errorf("1 TiB: got = %v, want 5", oneTB)
	}
}

func TestProgressStaysSilentWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, false)
	p.update("RUNNING", time.Second)
	p.clear()
	if buf.Len() != 0 {
		t.Errorf("got = %q, want nothing written", buf.String())
	}
}

func TestProgressOverwritesItsOwnLine(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, true)
	p.update("RUNNING", 1500*time.Millisecond)
	if got := buf.String(); !strings.Contains(got, "RUNNING 1.5s") {
		t.Errorf("got = %q, want it to contain %q", got, "RUNNING 1.5s")
	}
	if !strings.HasPrefix(buf.String(), "\r") {
		t.Error("got no carriage return, want the line rewritten in place")
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(2500 * time.Millisecond); got != "2.5s" {
		t.Errorf("got = %q, want %q", got, "2.5s")
	}
	if got := formatDuration(95 * time.Second); got != "1m35s" {
		t.Errorf("got = %q, want %q", got, "1m35s")
	}
}

func TestPricePerTBReadsTheEnvironment(t *testing.T) {
	t.Setenv(envPricePerTB, "7.25")
	if got := pricePerTB(); got != 7.25 {
		t.Errorf("got = %v, want 7.25", got)
	}
	t.Setenv(envPricePerTB, "not a number")
	if got := pricePerTB(); got != defaultPricePerTB {
		t.Errorf("invalid value: got = %v, want %v", got, defaultPricePerTB)
	}
}
