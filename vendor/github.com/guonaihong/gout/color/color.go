package color

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"os"
	"strings"
)

var (
	// NoColor 关闭颜色开关 本行代码来自github.com/fatih/color, 感谢fatih的付出
	NoColor = os.Getenv("TERM") == "dumb" ||
		(!isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()))
)

type attr int

const (
	// FgBlack 黑色
	FgBlack attr = iota + 30
	// FgRed 红色
	FgRed
	// FgGreen 绿色
	FgGreen
	// FgYellow 黄色
	FgYellow
	// FgBlue 蓝色
	FgBlue
	// FgMagenta 品红
	FgMagenta
	// FgCyan 青色
	FgCyan
	// FgWhite 白色
	FgWhite
)

const (
	// Purple 紫色
	Purple = 35
	// Blue 蓝色
	Blue = 34
)

// Color 着色核心数据结构
type Color struct {
	openColor bool
	attr      attr
}

// New 着色模块构造函数
func New(openColor bool, c ...attr) *Color {
	attr := attr(30)
	if len(c) > 0 {
		attr = c[0]
	}
	return &Color{openColor: openColor, attr: attr}
}

func (c *Color) set(buf *strings.Builder, attr attr) {
	if NoColor || !c.openColor {
		return
	}

	fmt.Fprintf(buf, "\x1b[%d;1m", attr)
}

func (c *Color) unset(buf *strings.Builder) {
	if NoColor || !c.openColor {
		return
	}

	fmt.Fprintf(buf, "\x1b[0m")
}

func (c *Color) color(a ...interface{}) string {
	var buf strings.Builder

	c.set(&buf, c.attr)

	fmt.Fprint(&buf, a...)
	c.unset(&buf)

	return buf.String()
}

func (c *Color) colorf(format string, a ...interface{}) string {
	var buf strings.Builder

	c.set(&buf, c.attr)

	fmt.Fprintf(&buf, format, a...)
	c.unset(&buf)

	return buf.String()
}

// Sbluef 蓝色函数
func (c *Color) Sbluef(format string, a ...interface{}) string {
	c.attr = Blue
	return c.colorf(format, a...)
}

// Sblue 蓝色函数
func (c *Color) Sblue(a ...interface{}) string {
	c.attr = Blue
	return c.color(a...)
}

// Spurplef 紫色函数, TODO删除该函数
func (c *Color) Spurplef(format string, a ...interface{}) string {
	c.attr = Purple
	return c.colorf(format, a...)
}

// Spurple 紫色函数
func (c *Color) Spurple(a ...interface{}) string {
	c.attr = Purple
	return c.color(a...)
}
