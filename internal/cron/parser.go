// Package cron provides a minimal 5-field cron expression parser.
// Supports: MIN HOUR DOM MON DOW
// Fields support: * (any), N (exact), N,M (list), */N (step), N-M (range), N-M/S (range+step)
// Sunday = 0 or 7.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Next returns the earliest time strictly after `after` that matches `expr`.
// Searches up to 2 years ahead; returns an error if no match is found (bad expr).
func Next(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron expr must have 5 fields, got %d: %q", len(fields), expr)
	}

	// Start from next minute
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.Add(2 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		// Check month first — biggest jump on mismatch
		if !fieldMatch(fields[3], int(t.Month()), 1, 12) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		// Check day-of-month
		if !fieldMatch(fields[2], t.Day(), 1, 31) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		// Check day-of-week
		if !fieldMatch(fields[4], int(t.Weekday()), 0, 6) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		// Check hour
		if !fieldMatch(fields[1], t.Hour(), 0, 23) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		// Check minute
		if !fieldMatch(fields[0], t.Minute(), 0, 59) {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no occurrence found within 2 years for: %q", expr)
}

// fieldMatch reports whether val satisfies the cron field expression for range [min, max].
func fieldMatch(field string, val, min, max int) bool {
	if field == "*" {
		return true
	}
	// Comma-separated list: "1,3,5"
	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			if fieldMatch(strings.TrimSpace(part), val, min, max) {
				return true
			}
		}
		return false
	}
	// Step: "*/N" or "M-N/S"
	if strings.Contains(field, "/") {
		parts := strings.SplitN(field, "/", 2)
		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return false
		}
		rMin, rMax := min, max
		if parts[0] != "*" {
			if strings.Contains(parts[0], "-") {
				rng := strings.SplitN(parts[0], "-", 2)
				rMin, _ = strconv.Atoi(rng[0])
				rMax, _ = strconv.Atoi(rng[1])
			} else {
				rMin, _ = strconv.Atoi(parts[0])
			}
		}
		if val < rMin || val > rMax {
			return false
		}
		return (val-rMin)%step == 0
	}
	// Range: "N-M"
	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)
		lo, e1 := strconv.Atoi(parts[0])
		hi, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil {
			return false
		}
		return val >= lo && val <= hi
	}
	// Exact value
	n, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	if n == 7 && max == 6 { // Sunday alias
		n = 0
	}
	return val == n
}
