package timestamp

import "time"

// FromTime returns a new millisecond timestamp from a time.
func FromTime(t time.Time) int64 {
	return t.Unix()*1000 + int64(t.Nanosecond())/int64(time.Millisecond)
}

// Time returns a new time.Time object from a millisecond timestamp.
func Time(ts int64) time.Time {
	return time.Unix(ts/1000, (ts%1000)*int64(time.Millisecond))
}
