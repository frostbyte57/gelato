package ui

import (
	"fmt"

	"gelato/internal/engine"
)

type statsView struct {
	valid   bool
	logs    string
	errors  string
	drops   string
	summary string
	legend  string
}

func (s *statsView) invalidate() {
	s.valid = false
}

func (s *statsView) ensure(snapshot engine.Snapshot) {
	if s.valid {
		return
	}
	logs := sparkline(snapshot.Stats.LogsPerSec)
	errors := sparkline(snapshot.Stats.ErrorsPerMin)
	drops := sparkline(snapshot.Stats.DropsPerMin)
	s.logs = "logs/s  " + logs
	s.errors = "errors  " + errors
	s.drops = "drops   " + drops
	s.summary = fmt.Sprintf("totals: logs=%d errors=%d drops=%d", sum(snapshot.Stats.LogsPerSec), snapshot.Errors, snapshot.Dropped)
	s.legend = "legend: / search  c clear  1 info 2 warn 3 error 4 debug  f follow"
	s.valid = true
}
