package terminal

import (
	"hash/fnv"
	"strings"
	"unicode/utf8"
)

const (
	defaultScrollbackCols = 80
	defaultScrollbackRows = 24
	maxScrollbackSeen     = 256
)

// NewScrollbackVTFilter builds a lightweight terminal scrollback filter.
func NewScrollbackVTFilter() OutputFilter {
	filter := &scrollbackVTFilter{
		stats: OutputFilterStats{FilterName: "scrollback-vt"},
	}
	filter.Resize(defaultScrollbackCols, defaultScrollbackRows)
	return filter
}

type vtState int

const (
	vtText vtState = iota
	vtEsc
	vtCSI
	vtOSC
	vtOSCEsc
	vtDCS
	vtDCSEsc
	vtPM
	vtPMEsc
	vtAPC
	vtAPCEsc
)

type scrollbackVTFilter struct {
	cols         int
	rows         int
	grid         [][]rune
	cursorRow    int
	cursorCol    int
	scrollTop    int
	scrollBottom int
	savedRow     int
	savedCol     int
	state        vtState
	csiBuf       []byte
	pending      []byte
	stats        OutputFilterStats
	seenHashes   map[uint64]struct{}
	seenOrder    []uint64
}

func (f *scrollbackVTFilter) Write(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	f.stats.InBytes += uint64(len(data))

	buf := append(f.pending, data...)
	f.pending = nil

	var out []byte
	for i := 0; i < len(buf); {
		b := buf[i]
		switch f.state {
		case vtText:
			switch b {
			case 0x1b:
				f.state = vtEsc
				i++
				continue
			case 0x9b:
				f.state = vtCSI
				f.csiBuf = f.csiBuf[:0]
				i++
				continue
			case 0x9d:
				f.state = vtOSC
				i++
				continue
			case 0x90:
				f.state = vtDCS
				i++
				continue
			case 0x9e:
				f.state = vtPM
				i++
				continue
			case 0x9f:
				f.state = vtAPC
				i++
				continue
			case '\n':
				out = append(out, f.lineFeed()...)
				i++
				continue
			case '\r':
				f.cursorCol = 0
				i++
				continue
			case '\t':
				f.advanceTab()
				i++
				continue
			case '\b':
				if f.cursorCol > 0 {
					f.cursorCol--
				}
				i++
				continue
			default:
				if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
					i++
					continue
				}
			}
			if b < utf8.RuneSelf {
				f.writeRune(rune(b))
				i++
				continue
			}
			if !utf8.FullRune(buf[i:]) {
				f.pending = append(f.pending, buf[i:]...)
				i = len(buf)
				continue
			}
			r, size := utf8.DecodeRune(buf[i:])
			if r == utf8.RuneError && size == 1 {
				f.writeRune(utf8.RuneError)
				i++
				continue
			}
			f.writeRune(r)
			i += size
		case vtEsc:
			switch b {
			case '[':
				f.state = vtCSI
				f.csiBuf = f.csiBuf[:0]
			case ']':
				f.state = vtOSC
			case 'P':
				f.state = vtDCS
			case '^':
				f.state = vtPM
			case '_':
				f.state = vtAPC
			case '7':
				f.savedRow = f.cursorRow
				f.savedCol = f.cursorCol
				f.state = vtText
			case '8':
				f.cursorRow = f.savedRow
				f.cursorCol = f.savedCol
				f.clampCursor()
				f.state = vtText
			default:
				f.state = vtText
			}
			i++
		case vtCSI:
			if b >= 0x40 && b <= 0x7e {
				f.handleCSI(b, f.csiBuf)
				f.csiBuf = f.csiBuf[:0]
				f.state = vtText
				i++
				continue
			}
			f.csiBuf = append(f.csiBuf, b)
			i++
		case vtOSC:
			if b == 0x07 {
				f.state = vtText
			} else if b == 0x1b {
				f.state = vtOSCEsc
			}
			i++
		case vtOSCEsc:
			if b == '\\' {
				f.state = vtText
			} else {
				f.state = vtOSC
			}
			i++
		case vtDCS:
			if b == 0x1b {
				f.state = vtDCSEsc
			}
			i++
		case vtDCSEsc:
			if b == '\\' {
				f.state = vtText
			} else {
				f.state = vtDCS
			}
			i++
		case vtPM:
			if b == 0x1b {
				f.state = vtPMEsc
			}
			i++
		case vtPMEsc:
			if b == '\\' {
				f.state = vtText
			} else {
				f.state = vtPM
			}
			i++
		case vtAPC:
			if b == 0x1b {
				f.state = vtAPCEsc
			}
			i++
		case vtAPCEsc:
			if b == '\\' {
				f.state = vtText
			} else {
				f.state = vtAPC
			}
			i++
		}
	}
	if len(out) > 0 {
		f.stats.OutBytes += uint64(len(out))
		if len(out) < len(data) {
			f.stats.DroppedBytes += uint64(len(data) - len(out))
		}
	}
	return out
}

func (f *scrollbackVTFilter) Flush() []byte {
	if f.rows == 0 || f.cols == 0 {
		return nil
	}
	var out []byte
	for row := f.scrollTop; row <= f.scrollBottom; row++ {
		line := f.renderLine(f.grid[row])
		if emitted := f.emitLine(line); len(emitted) > 0 {
			out = append(out, emitted...)
		}
	}
	if len(out) > 0 {
		f.stats.OutBytes += uint64(len(out))
	}
	return out
}

func (f *scrollbackVTFilter) Resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	f.cols = int(cols)
	f.rows = int(rows)
	f.grid = make([][]rune, f.rows)
	for i := range f.grid {
		f.grid[i] = f.blankLine()
	}
	f.scrollTop = 0
	f.scrollBottom = f.rows - 1
	f.cursorRow = 0
	f.cursorCol = 0
	f.savedRow = 0
	f.savedCol = 0
	f.state = vtText
	f.pending = nil
	f.csiBuf = nil
	f.seenHashes = make(map[uint64]struct{}, maxScrollbackSeen)
	f.seenOrder = nil
}

func (f *scrollbackVTFilter) Reset() {
	f.Resize(uint16(f.cols), uint16(f.rows))
	f.stats = OutputFilterStats{FilterName: "scrollback-vt"}
}

func (f *scrollbackVTFilter) Stats() OutputFilterStats {
	return f.stats
}

func (f *scrollbackVTFilter) blankLine() []rune {
	line := make([]rune, f.cols)
	for i := range line {
		line[i] = ' '
	}
	return line
}

func (f *scrollbackVTFilter) writeRune(r rune) {
	if f.rows == 0 || f.cols == 0 {
		return
	}
	f.clampCursor()
	f.grid[f.cursorRow][f.cursorCol] = r
	if f.cursorCol >= f.cols-1 {
		f.cursorCol = 0
		f.cursorRow++
		if f.cursorRow > f.scrollBottom {
			f.cursorRow = f.scrollBottom
			f.scrollUp()
		}
		return
	}
	f.cursorCol++
}

func (f *scrollbackVTFilter) lineFeed() []byte {
	f.cursorCol = 0
	f.cursorRow++
	if f.cursorRow > f.scrollBottom {
		f.cursorRow = f.scrollBottom
		return f.scrollUp()
	}
	return nil
}

func (f *scrollbackVTFilter) scrollUp() []byte {
	line := f.grid[f.scrollTop]
	for i := f.scrollTop; i < f.scrollBottom; i++ {
		f.grid[i] = f.grid[i+1]
	}
	f.grid[f.scrollBottom] = f.blankLine()
	return f.emitLine(f.renderLine(line))
}

func (f *scrollbackVTFilter) advanceTab() {
	next := ((f.cursorCol / 8) + 1) * 8
	if next >= f.cols {
		f.cursorCol = 0
		f.cursorRow++
		if f.cursorRow > f.scrollBottom {
			f.cursorRow = f.scrollBottom
			f.scrollUp()
		}
		return
	}
	f.cursorCol = next
}

func (f *scrollbackVTFilter) clampCursor() {
	if f.cursorRow < 0 {
		f.cursorRow = 0
	}
	if f.cursorRow >= f.rows {
		f.cursorRow = f.rows - 1
	}
	if f.cursorCol < 0 {
		f.cursorCol = 0
	}
	if f.cursorCol >= f.cols {
		f.cursorCol = f.cols - 1
	}
}

func (f *scrollbackVTFilter) handleCSI(final byte, params []byte) {
	values := parseCSIParams(params)
	switch final {
	case 'A':
		f.moveCursor(-paramOrDefault(values, 1), 0)
	case 'B':
		f.moveCursor(paramOrDefault(values, 1), 0)
	case 'C':
		f.moveCursor(0, paramOrDefault(values, 1))
	case 'D':
		f.moveCursor(0, -paramOrDefault(values, 1))
	case 'H', 'f':
		row := paramOrDefault(values, 1)
		col := 1
		if len(values) > 1 {
			col = values[1]
		}
		f.cursorRow = clampIndex(row-1, 0, f.rows-1)
		f.cursorCol = clampIndex(col-1, 0, f.cols-1)
	case 'J':
		mode := paramOrDefault(values, 0)
		f.clearScreen(mode)
	case 'K':
		mode := paramOrDefault(values, 0)
		f.clearLine(mode)
	case 'r':
		f.setScrollRegion(values)
	case 's':
		f.savedRow = f.cursorRow
		f.savedCol = f.cursorCol
	case 'u':
		f.cursorRow = f.savedRow
		f.cursorCol = f.savedCol
		f.clampCursor()
	default:
	}
}

func (f *scrollbackVTFilter) moveCursor(rowDelta, colDelta int) {
	f.cursorRow += rowDelta
	f.cursorCol += colDelta
	f.clampCursor()
}

func (f *scrollbackVTFilter) clearLine(mode int) {
	f.clampCursor()
	line := f.grid[f.cursorRow]
	switch mode {
	case 0:
		for i := f.cursorCol; i < f.cols; i++ {
			line[i] = ' '
		}
	case 1:
		for i := 0; i <= f.cursorCol; i++ {
			line[i] = ' '
		}
	case 2:
		for i := 0; i < f.cols; i++ {
			line[i] = ' '
		}
	}
}

func (f *scrollbackVTFilter) clearScreen(mode int) {
	f.clampCursor()
	switch mode {
	case 0:
		f.clearLine(0)
		for row := f.cursorRow + 1; row < f.rows; row++ {
			f.grid[row] = f.blankLine()
		}
	case 1:
		for row := 0; row < f.cursorRow; row++ {
			f.grid[row] = f.blankLine()
		}
		f.clearLine(1)
	case 2:
		for row := 0; row < f.rows; row++ {
			f.grid[row] = f.blankLine()
		}
	}
}

func (f *scrollbackVTFilter) setScrollRegion(values []int) {
	if len(values) == 0 {
		f.scrollTop = 0
		f.scrollBottom = f.rows - 1
		f.cursorRow = 0
		f.cursorCol = 0
		return
	}
	top := values[0]
	bottom := values[len(values)-1]
	if top < 1 || bottom < 1 || top >= bottom || bottom > f.rows {
		f.scrollTop = 0
		f.scrollBottom = f.rows - 1
		f.cursorRow = 0
		f.cursorCol = 0
		return
	}
	f.scrollTop = top - 1
	f.scrollBottom = bottom - 1
	f.cursorRow = f.scrollTop
	f.cursorCol = 0
}

func (f *scrollbackVTFilter) renderLine(line []rune) string {
	rendered := strings.TrimRight(string(line), " ")
	return rendered
}

func (f *scrollbackVTFilter) emitLine(line string) []byte {
	hash := hashLine(line)
	if _, ok := f.seenHashes[hash]; ok {
		return nil
	}
	f.seenHashes[hash] = struct{}{}
	f.seenOrder = append(f.seenOrder, hash)
	if len(f.seenOrder) > maxScrollbackSeen {
		oldest := f.seenOrder[0]
		f.seenOrder = f.seenOrder[1:]
		delete(f.seenHashes, oldest)
	}
	return []byte(line + "\n")
}

func hashLine(line string) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(line))
	return hasher.Sum64()
}

func parseCSIParams(params []byte) []int {
	values := []int{}
	value := -1
	sawSep := false
	for _, b := range params {
		if b >= '0' && b <= '9' {
			if value < 0 {
				value = 0
			}
			value = value*10 + int(b-'0')
			sawSep = false
			continue
		}
		if b == ';' {
			if value < 0 {
				values = append(values, 0)
			} else {
				values = append(values, value)
				value = -1
			}
			sawSep = true
			continue
		}
	}
	if value >= 0 {
		values = append(values, value)
	} else if sawSep {
		values = append(values, 0)
	}
	return values
}

func paramOrDefault(values []int, fallback int) int {
	if len(values) == 0 {
		return fallback
	}
	if values[0] == 0 {
		return fallback
	}
	return values[0]
}

func clampIndex(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
