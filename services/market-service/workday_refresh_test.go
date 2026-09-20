package main

import (
	"errors"
	"testing"
	"time"
)

type fakeWorkdayCalendar struct {
	workdays map[string]bool
	update   func() error
	calls    int
}

func (f *fakeWorkdayCalendar) Is(target time.Time) bool {
	return f.workdays[target.Format("20060102")]
}

func (f *fakeWorkdayCalendar) Update() error {
	f.calls++
	if f.update != nil {
		return f.update()
	}
	return nil
}

func TestWorkdayStatusRefreshesCurrentWeekday(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 9, 21, 14, 40, 0, 0, location)
	calendar := &fakeWorkdayCalendar{workdays: map[string]bool{}}
	calendar.update = func() error {
		calendar.workdays["20260921"] = true
		return nil
	}

	isWorkday, err := workdayStatus(calendar, now, now)
	if err != nil {
		t.Fatalf("workdayStatus returned error: %v", err)
	}
	if !isWorkday || calendar.calls != 1 {
		t.Fatalf("isWorkday=%v updateCalls=%d, want true and 1", isWorkday, calendar.calls)
	}
}

func TestWorkdayStatusDoesNotRefreshFutureOrWeekend(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	calendar := &fakeWorkdayCalendar{workdays: map[string]bool{}}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, location)

	for _, target := range []time.Time{
		time.Date(2026, 9, 21, 0, 0, 0, 0, location),
		time.Date(2026, 9, 20, 0, 0, 0, 0, location),
	} {
		isWorkday, err := workdayStatus(calendar, target, now)
		if err != nil || isWorkday {
			t.Fatalf("target=%v isWorkday=%v err=%v, want false and nil", target, isWorkday, err)
		}
	}
	if calendar.calls != 0 {
		t.Fatalf("updateCalls=%d, want 0", calendar.calls)
	}
}

func TestWorkdayStatusReturnsRefreshError(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, location)
	wantErr := errors.New("tdx unavailable")
	calendar := &fakeWorkdayCalendar{
		workdays: map[string]bool{},
		update: func() error {
			return wantErr
		},
	}

	if _, err := workdayStatus(calendar, now, now); !errors.Is(err, wantErr) {
		t.Fatalf("error=%v, want %v", err, wantErr)
	}
}
