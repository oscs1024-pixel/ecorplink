package gui

import (
	"os"
	"strings"
)

func (s *Service) ReadLog(req LogRequest) LogChunk {
	path := expandHome(s.logPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LogChunk{OK: true, Path: path, Text: ""}
		}
		return LogChunk{OK: false, Path: path, Error: err.Error()}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if req.Query != "" {
		filtered := lines[:0]
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), strings.ToLower(req.Query)) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	if req.Lines <= 0 {
		req.Lines = 200
	}
	if len(lines) > req.Lines {
		lines = lines[len(lines)-req.Lines:]
	}
	return LogChunk{OK: true, Path: path, Text: strings.Join(lines, "\n")}
}
