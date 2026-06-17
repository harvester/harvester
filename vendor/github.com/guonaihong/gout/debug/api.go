package debug

import (
	"io"
	"os"
)

// 默认，不需要调用
func DefaultDebug(o *Options) {
	o.Color = true
	o.Debug = true
	o.Write = os.Stdout
}

// NoColor Turn off color highlight debug mode
func NoColor() Apply {
	return Func(func(o *Options) {
		o.Color = false
		o.Debug = true
		o.Trace = true
		o.Write = os.Stdout
	})
}

type file struct{ fileName string }

func (f *file) Write(p []byte) (n int, err error) {
	fd, err := os.OpenFile(f.fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}

	defer fd.Close()
	return fd.Write(p)
}

// 第一个参数是要写入的文件名, 第二个参数是否颜色高亮
func ToFile(fileName string, color bool) Apply {
	return Func(func(o *Options) {
		o.Color = color
		o.Debug = true
		o.Trace = false
		o.Write = &file{fileName: fileName}
	})
}

// 第一个参数是要写入的io.Writer对象， 第二个参数是否颜色高亮
func ToWriter(w io.Writer, color bool) Apply {
	return Func(func(o *Options) {
		o.Color = color
		o.Debug = true
		o.Trace = false
		o.Write = w
	})
}

func OnlyTraceFlag() Apply {
	return Func(func(o *Options) {
		o.Trace = false
	})
}

// trace信息格式化成json输出至标准输出
func TraceJSON() Apply {
	return TraceJSONToWriter(os.Stdout)
}

// trace信息格式化成json输出至w
func TraceJSONToWriter(w io.Writer) Apply {
	return Func(func(o *Options) {
		o.Color = false
		o.Debug = false
		o.Trace = true
		o.FormatTraceJSON = true
		o.Write = w
	})
}

// 打开Trace()
func Trace() Apply {
	return Func(func(o *Options) {
		o.Color = true
		o.Trace = true
		o.Write = os.Stdout
	})
}
