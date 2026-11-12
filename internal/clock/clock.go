// Package clock 提供可注入的时间源。
// 业务中所有时间获取必须经过 Clock，禁止直接调用 time.Now()，
// 以便教学时钟（时间加速/平移）与单元测试。
package clock

import "time"

// Clock 业务时间源接口。
type Clock interface {
	Now() time.Time
}

// System 系统真实时间。
type System struct{}

func (System) Now() time.Time { return time.Now() }

// Fixed 固定时间，用于测试。
type Fixed struct {
	T time.Time
}

func (f Fixed) Now() time.Time { return f.T }

// Shift 在真实时间基础上加固定偏移，用于教学时钟演示（如把"明天"提前）。
type Shift struct {
	Offset time.Duration
}

func (s Shift) Now() time.Time { return time.Now().Add(s.Offset) }
