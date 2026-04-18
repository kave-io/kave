package mappers

import (
	"time"

	"github.com/kave-io/kave/core/pkg/timex"
)

func msSinceEpoch() int64 {
	return time.Now().UnixMilli()
}

func msToTimingValue(ms int64) timex.MS {
	return timex.MS(ms)
}

func ptrMSToTiming(ms *int64) *timex.MS {
	if ms == nil {
		return nil
	}
	v := timex.MS(*ms)
	return &v
}

func ptrMSToTimingValue(ms *int64) timex.MS {
	if ms == nil {
		return timex.MS(0)
	}
	return timex.MS(*ms)
}

func ptrTimingToMS(ms *timex.MS) *int64 {
	if ms == nil {
		return nil
	}
	v := int64(*ms)
	return &v
}

func timingToMS(ms timex.MS) int64 {
	return int64(ms)
}

func ptrMS(ms timex.MS) *int64 {
	if ms == 0 {
		return nil
	}
	v := int64(ms)
	return &v
}

func ptrMSFromTiming(ms timex.MS) *int64 {
	return ptrMS(ms)
}
