package terminal

// NewANSIStripFilter removes ANSI escape sequences and control bytes.
func NewANSIStripFilter() OutputFilter {
	return &ansiStripFilter{
		stats: OutputFilterStats{FilterName: "ansi-strip"},
	}
}

type ansiState int

const (
	ansiText ansiState = iota
	ansiEsc
	ansiCSI
	ansiOSC
	ansiDCS
	ansiPM
	ansiAPC
	ansiOSCEsc
	ansiDCSEsc
	ansiPMEsc
	ansiAPCEsc
)

type ansiStripFilter struct {
	state ansiState
	stats OutputFilterStats
}

func (f *ansiStripFilter) Write(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	f.stats.InBytes += uint64(len(data))

	out := make([]byte, 0, len(data))
	var dropped uint64
	for _, b := range data {
		switch f.state {
		case ansiText:
			switch b {
			case 0x1b:
				f.state = ansiEsc
				dropped++
			case 0x9b:
				f.state = ansiCSI
				dropped++
			case 0x9d:
				f.state = ansiOSC
				dropped++
			case 0x90:
				f.state = ansiDCS
				dropped++
			case 0x9e:
				f.state = ansiPM
				dropped++
			case 0x9f:
				f.state = ansiAPC
				dropped++
			case '\n', '\r', '\t':
				out = append(out, b)
			default:
				if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
					dropped++
					continue
				}
				out = append(out, b)
			}
		case ansiEsc:
			switch b {
			case '[':
				f.state = ansiCSI
			case ']':
				f.state = ansiOSC
			case 'P':
				f.state = ansiDCS
			case '^':
				f.state = ansiPM
			case '_':
				f.state = ansiAPC
			default:
				f.state = ansiText
			}
			dropped++
		case ansiCSI:
			if b >= 0x40 && b <= 0x7e {
				f.state = ansiText
			}
			dropped++
		case ansiOSC:
			if b == 0x07 {
				f.state = ansiText
			} else if b == 0x1b {
				f.state = ansiOSCEsc
			}
			dropped++
		case ansiOSCEsc:
			if b == '\\' {
				f.state = ansiText
			} else {
				f.state = ansiOSC
			}
			dropped++
		case ansiDCS:
			if b == 0x1b {
				f.state = ansiDCSEsc
			}
			dropped++
		case ansiDCSEsc:
			if b == '\\' {
				f.state = ansiText
			} else {
				f.state = ansiDCS
			}
			dropped++
		case ansiPM:
			if b == 0x1b {
				f.state = ansiPMEsc
			}
			dropped++
		case ansiPMEsc:
			if b == '\\' {
				f.state = ansiText
			} else {
				f.state = ansiPM
			}
			dropped++
		case ansiAPC:
			if b == 0x1b {
				f.state = ansiAPCEsc
			}
			dropped++
		case ansiAPCEsc:
			if b == '\\' {
				f.state = ansiText
			} else {
				f.state = ansiAPC
			}
			dropped++
		}
	}
	f.stats.OutBytes += uint64(len(out))
	f.stats.DroppedBytes += dropped
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f *ansiStripFilter) Flush() []byte {
	return nil
}

func (f *ansiStripFilter) Resize(cols, rows uint16) {}

func (f *ansiStripFilter) Reset() {
	f.state = ansiText
	f.stats = OutputFilterStats{FilterName: "ansi-strip"}
}

func (f *ansiStripFilter) Stats() OutputFilterStats {
	return f.stats
}
