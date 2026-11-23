package domain

// PeakRateWindow 是订阅分组的一个高峰时段配置。
//   - Start/End 为服务器全局时区下的 HH:MM，区间左闭右开 [Start, End)，不支持跨天；
//   - Multiplier 为高峰期间叠加到文本 token 倍率上的因子，>= 0；
//   - Days 为生效的星期，0=周日 … 6=周六；空数组/缺省表示每天生效。
type PeakRateWindow struct {
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Multiplier float64 `json:"multiplier"`
	Days       []int   `json:"days,omitempty"`
}
