package terminal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultHistoryLines = 2000

func tailLines(lines []string, maxLines int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	return lines[len(lines)-maxLines:]
}

func mergeHistoryLines(fileLines, bufferLines []string, maxLines int) []string {
	if len(fileLines) == 0 {
		return tailLines(bufferLines, maxLines)
	}
	if len(bufferLines) == 0 {
		return tailLines(fileLines, maxLines)
	}

	maxOverlap := len(fileLines)
	if len(bufferLines) < maxOverlap {
		maxOverlap = len(bufferLines)
	}
	overlap := 0
	for size := maxOverlap; size > 0; size-- {
		matched := true
		for i := 0; i < size; i++ {
			if fileLines[len(fileLines)-size+i] != bufferLines[i] {
				matched = false
				break
			}
		}
		if matched {
			overlap = size
			break
		}
	}

	combined := make([]string, 0, len(fileLines)+len(bufferLines)-overlap)
	combined = append(combined, fileLines[:len(fileLines)-overlap]...)
	combined = append(combined, bufferLines...)
	return tailLines(combined, maxLines)
}

func readLastLines(path string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		return []string{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return []string{}, nil
	}

	const chunkSize = 4096
	var (
		offset       = info.Size()
		newlineCount = 0
		buffer       []byte
	)

	for offset > 0 && newlineCount <= maxLines {
		readSize := int64(chunkSize)
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
		chunk := make([]byte, readSize)
		n, err := file.Read(chunk)
		if n > 0 {
			chunk = chunk[:n]
			newlineCount += bytes.Count(chunk, []byte{'\n'})
			buffer = append(chunk, buffer...)
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
	}

	lines := strings.Split(string(buffer), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return tailLines(lines, maxLines), nil
}

func readLastLinesBefore(path string, maxLines int, endOffset int64) ([]string, int64, error) {
	if maxLines <= 0 {
		return []string{}, endOffset, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if info.Size() == 0 {
		return []string{}, 0, nil
	}

	if endOffset <= 0 || endOffset > info.Size() {
		endOffset = info.Size()
	}
	if endOffset == 0 {
		return []string{}, 0, nil
	}

	const chunkSize = 4096
	var (
		offset       = endOffset
		newlineCount = 0
		buffer       []byte
	)

	for offset > 0 && newlineCount <= maxLines {
		readSize := int64(chunkSize)
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return nil, 0, err
		}
		chunk := make([]byte, readSize)
		n, err := file.Read(chunk)
		if n > 0 {
			chunk = chunk[:n]
			newlineCount += bytes.Count(chunk, []byte{'\n'})
			buffer = append(chunk, buffer...)
		}
		if err != nil && err != io.EOF {
			return nil, 0, err
		}
	}

	lines := strings.Split(string(buffer), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= maxLines {
		return lines, offset, nil
	}

	startIndex := len(lines) - maxLines
	bytesToSkip := 0
	for i := 0; i < startIndex; i++ {
		bytesToSkip += len(lines[i]) + 1
	}
	startOffset := offset + int64(bytesToSkip)
	return lines[startIndex:], startOffset, nil
}

func latestSessionLogPath(dir, terminalID string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	prefix := terminalID + "-"
	var latest string
	var latestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latest == "" || info.ModTime().After(latestTime) {
			latest = name
			latestTime = info.ModTime()
		}
	}

	if latest == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(dir, latest), nil
}
