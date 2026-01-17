package pine

import (
	"time"
)

func timerUntilMidnight() *time.Timer {
	now := time.Now()
	loc := now.Location()

	// Next midnight
	nextMidnight := time.Date(
		now.Year(),
		now.Month(),
		now.Day()+1,
		0, 0, 0, 0,
		loc,
	)

	return time.NewTimer(time.Until(nextMidnight))
}
