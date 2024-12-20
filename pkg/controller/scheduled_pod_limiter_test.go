package controller

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/require"
)

func TestShouldLimitPods(t *testing.T) {
	tests := map[string]struct {
		startSchedule   string
		stopSchedule    string
		time            time.Time
		shouldLimitPods bool
	}{
		"startAndStopEveryMinute": {
			startSchedule:   "* * * * *",
			stopSchedule:    "* * * * *",
			time:            time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			shouldLimitPods: false,
		},
		"startEvenMinutesStopOddMinutesAt30SecondsPastEvenMinute": {
			startSchedule:   "*/2 * * * *",
			stopSchedule:    "1-59/2 * * * *",
			time:            time.Date(2024, 1, 1, 0, 0, 30, 0, time.UTC),
			shouldLimitPods: false,
		},
		"startEvenMinutesStopOddMinutesAt30SecondsPastOddMinute": {
			startSchedule:   "*/2 * * * *",
			stopSchedule:    "1-59/2 * * * *",
			time:            time.Date(2024, 1, 1, 0, 1, 30, 0, time.UTC),
			shouldLimitPods: true,
		},
		"startMondayMorningStopFridayEveningDuringWeek": {
			startSchedule: "0 8 * * Mon",
			stopSchedule:  "0 16 * * Fri",
			// Wednesday, January 10, 2024 8:00:00 AM
			time:            time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC),
			shouldLimitPods: false,
		},
		"startMondayMorningStopFridayEveningDuringWeekend": {
			startSchedule: "0 8 * * Mon",
			stopSchedule:  "0 16 * * Fri",
			// Sunday, January 14, 2024 10:00:00 PM
			time:            time.Date(2024, 1, 14, 22, 0, 0, 0, time.UTC),
			shouldLimitPods: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			startSchedule, err := cron.ParseStandard(test.startSchedule)
			require.Nil(t, err)
			stopSchedule, err := cron.ParseStandard(test.stopSchedule)
			require.Nil(t, err)
			s := &scheduledPodLimiter{
				startSchedule: startSchedule,
				stopSchedule:  stopSchedule,
			}
			require.Equal(t, test.shouldLimitPods, s.shouldLimitPods(test.time))
		})
	}
}
