package widgets

import (
	"strings"

	"github.com/jroimartin/gocui"
)

func ArrowUp(_ *gocui.Gui, v *gocui.View) error {
	if v == nil || isAtTop(v) {
		return nil
	}

	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy-1); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return nil
}

func ArrowDown(_ *gocui.Gui, v *gocui.View) error {
	if v == nil || isAtEnd(v) {
		return nil
	}
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy+1); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}

	return nil
}

func isAtTop(v *gocui.View) bool {
	_, cy := v.Cursor()
	return cy == 0
}

func isAtEnd(v *gocui.View) bool {
	_, cy := v.Cursor()
	lines := len(v.BufferLines())
	if lines < 2 || cy == lines-2 {
		return true
	}
	return false
}

// wrapText will insert the separator in a string so that it doesn't exceed
// the specified number of columns.
func wrapText(text string, columns int, separator string) string {
	if len(text) < columns {
		return text
	}
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == ' ' && i < columns {
			return text[:i] + separator + wrapText(text[i+1:], columns, separator)
		}
	}
	return text
}

// formatContent will format the content string based on the provided parameters.
// If wrap is true, it will insert line breaks to ensure no line exceeds maxColumns.
// If truncate is true, it will limit the number of lines to maxRows, appending "..." if truncation occurs.
func formatContent(content string, maxColumns, maxRows int, wrap, truncate bool) string {
	separator := "\n"
	if wrap {
		lines := strings.Split(content, separator)
		content = ""
		for i, line := range lines {
			if i > 0 {
				content += separator
			}
			if len(line) < maxColumns {
				content += line
			} else {
				content += wrapText(line, maxColumns, separator)
			}
		}
	}
	if truncate {
		truncateSuffix := " ..."
		lenTruncateSuffix := len(truncateSuffix)
		lines := strings.Split(content, separator)
		if len(lines) > maxRows {
			cutLinesAt := maxRows - 1
			lastLine := lines[cutLinesAt]
			lines = lines[:cutLinesAt]
			if (maxColumns > 0) && (len(lastLine)+lenTruncateSuffix > maxColumns) {
				// Just cut it off here without checking whether the text to
				// be displayed is still legible or not; you could search for
				// the last space here, but that would only complicate things.
				cutLastLineAt := maxColumns - lenTruncateSuffix
				lastLine = lastLine[:cutLastLineAt]
			}
			lines = append(lines, lastLine+truncateSuffix)
		}
		content = strings.Join(lines, separator)
	}
	return content
}
