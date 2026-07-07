package core

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Date is a calendar date (no time, no zone). COGO reasons in whole days, so a
// date is a date — modeling it as time.Time would invite timezone bugs in the
// freshness math. It serializes as a plain YYYY-MM-DD scalar.
type Date struct{ t time.Time }

const dateLayout = "2006-01-02"

// NewDate builds a Date from year/month/day.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses "YYYY-MM-DD"; an empty string yields the zero Date.
func ParseDate(s string) (Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Date{}, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: want YYYY-MM-DD", s)
	}
	return Date{t.UTC()}, nil
}

// MustDate is ParseDate that panics on error — for tests and constants.
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Date) IsZero() bool       { return d.t.IsZero() }
func (d Date) AddDays(n int) Date { return Date{d.t.AddDate(0, 0, n)} }
func (d Date) After(o Date) bool  { return d.t.After(o.t) }
func (d Date) Before(o Date) bool { return d.t.Before(o.t) }
func (d Date) Time() time.Time    { return d.t }

// DaysSince returns whole days from o to d (negative if o is in the future).
func (d Date) DaysSince(o Date) int {
	if d.t.IsZero() || o.t.IsZero() {
		return 0
	}
	return int(d.t.Sub(o.t).Hours() / 24)
}

func (d Date) String() string {
	if d.t.IsZero() {
		return ""
	}
	return d.t.Format(dateLayout)
}

// MarshalYAML emits the date as a plain YYYY-MM-DD scalar (zero -> null).
func (d Date) MarshalYAML() (any, error) {
	if d.t.IsZero() {
		return nil, nil
	}
	return d.t.Format(dateLayout), nil
}

// UnmarshalYAML accepts YYYY-MM-DD, tolerating a full timestamp (first 10 chars).
func (d *Date) UnmarshalYAML(value *yaml.Node) error {
	s := strings.TrimSpace(value.Value)
	if s == "" || s == "null" || s == "~" {
		return nil
	}
	if len(s) >= 10 {
		if t, err := time.Parse(dateLayout, s[:10]); err == nil {
			d.t = t.UTC()
			return nil
		}
	}
	return fmt.Errorf("invalid date %q: want YYYY-MM-DD", s)
}
