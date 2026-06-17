package setting

import (
	"time"

	"github.com/guonaihong/gout/debug"
)

// 设置
type Setting struct {
	// debug相关字段
	debug.Options
	// 控制是否使用空值
	NotIgnoreEmpty bool

	//是否自动加ContentType
	NoAutoContentType bool
	//超时时间
	Timeout time.Duration

	UseChunked bool
}

// 使用chunked数据
func (s *Setting) Chunked() {
	s.UseChunked = true
}

func (s *Setting) SetTimeout(d time.Duration) {
	s.Timeout = d
}

func (s *Setting) SetDebug(b bool) {
	s.Debug = b
}

func (s *Setting) Reset() {
	s.NotIgnoreEmpty = false
	s.NoAutoContentType = false
	//s.TimeoutIndex = 0
	s.Timeout = time.Duration(0)
}
