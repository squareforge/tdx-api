package main

import (
	"sync"
	"time"
)

type workdayCalendar interface {
	Is(time.Time) bool
	Update() error
}

var workdayRefreshMu sync.Mutex

// workdayStatus refreshes an unresolved current weekday on demand. The TDX
// workday calendar is derived from index bars, so a service started before the
// session cannot know today's status until an intraday bar becomes available.
func workdayStatus(calendar workdayCalendar, target, now time.Time) (bool, error) {
	if calendar.Is(target) {
		return true, nil
	}
	if !sameCalendarDate(target, now) || target.Weekday() == time.Saturday || target.Weekday() == time.Sunday {
		return false, nil
	}

	workdayRefreshMu.Lock()
	defer workdayRefreshMu.Unlock()
	if calendar.Is(target) {
		return true, nil
	}
	if err := calendar.Update(); err != nil {
		return false, err
	}
	return calendar.Is(target), nil
}

func sameCalendarDate(left, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}
