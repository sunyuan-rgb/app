package model

import (
	"testing"
	"time"
)

func TestBizDate_BeijingAttribution(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		// UTC 2026-11-10 16:00 = 北京 11-11 00:00（跨日边界：归新的一天）
		{"utc16点跨到北京次日", time.Date(2026, 11, 10, 16, 0, 0, 0, time.UTC), "2026-11-11"},
		// UTC 2026-11-10 15:59 = 北京 11-10 23:59（仍属前一天）
		{"北京23点59分", time.Date(2026, 11, 10, 15, 59, 0, 0, time.UTC), "2026-11-10"},
		{"北京当天中午", time.Date(2026, 11, 11, 4, 0, 0, 0, time.UTC), "2026-11-11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BizDate(c.t); got != c.want {
				t.Fatalf("BizDate(%v) = %s, want %s", c.t, got, c.want)
			}
		})
	}
}

func TestBizDate_InputWithOffsetZone(t *testing.T) {
	// 带 +08:00 的输入直接换算，不受本地时区影响
	tl := time.FixedZone("TEST+8", 8*3600)
	in := time.Date(2026, 11, 11, 23, 59, 0, 0, tl)
	if got := BizDate(in); got != "2026-11-11" {
		t.Fatalf("got %s", got)
	}
}
