package athq

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	defaultPricePerTB = 5.0
	envPricePerTB     = "ATHQ_PRICE_PER_TB"
	bytesPerTB        = 1024.0 * 1024.0 * 1024.0 * 1024.0
)

// progress draws a single self overwriting status line on stderr while a query
// is running. It stays silent when stderr is not a terminal so that logs and
// pipes are not polluted.
type progress struct {
	w       io.Writer
	enabled bool
	lastLen int
}

func newProgress(w io.Writer, enabled bool) *progress {
	return &progress{w: w, enabled: enabled}
}

func newStderrProgress() *progress {
	return newProgress(os.Stderr, isTerminal(os.Stderr))
}

// newDiscardProgress is for callers that own the screen themselves, such as
// the TUI.
func newDiscardProgress() *progress {
	return newProgress(io.Discard, false)
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func terminalWidth(f *os.File) int {
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

func (p *progress) update(state string, elapsed time.Duration) {
	if !p.enabled {
		return
	}
	line := fmt.Sprintf("%s %s", state, formatDuration(elapsed))
	pad := ""
	if n := p.lastLen - len(line); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	_, _ = fmt.Fprintf(p.w, "\r%s%s", line, pad)
	p.lastLen = len(line)
}

func (p *progress) clear() {
	if !p.enabled || p.lastLen == 0 {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastLen))
	p.lastLen = 0
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// pricePerTB is the price used for the cost estimate. It differs per region so
// it can be overridden with an environment variable.
func pricePerTB() float64 {
	if v := os.Getenv(envPricePerTB); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	return defaultPricePerTB
}

// estimateCost returns the rough charge for the scanned bytes. Athena bills a
// minimum of 10 MB per query.
func estimateCost(scanned int64, price float64) float64 {
	const minimumBilled = 10 * 1024 * 1024
	if scanned < minimumBilled {
		scanned = minimumBilled
	}
	return float64(scanned) / bytesPerTB * price
}
