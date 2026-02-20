package engine

import (
	"strings"

	"gelato/internal/model"
)

type filterCache struct {
	version   uint64
	sourceKey string
	levelMask uint32
	search    string
	lines     []string
	levels    []model.Level
}

func filterLines(lines []string, levels []model.Level, filter string) ([]string, []model.Level) {
	if filter == "" {
		return lines, levels
	}
	filtered := make([]string, 0, len(lines))
	filteredLevels := make([]model.Level, 0, len(lines))
	needle := strings.ToLower(filter)
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			filtered = append(filtered, line)
			filteredLevels = append(filteredLevels, levels[i])
		}
	}
	return filtered, filteredLevels
}

func filterLinesByLevel(lines []string, levels []model.Level, levelMask uint32) ([]string, []model.Level) {
	if levelMask == model.LevelMaskAll {
		return lines, levels
	}
	filtered := make([]string, 0, len(lines))
	filteredLevels := make([]model.Level, 0, len(lines))
	for i := range lines {
		level := levels[i]
		if levelMask&levelToMask(level) != 0 {
			filtered = append(filtered, lines[i])
			filteredLevels = append(filteredLevels, level)
		}
	}
	return filtered, filteredLevels
}

func levelToMask(level model.Level) uint32 {
	switch level {
	case model.LevelDebug:
		return model.LevelMaskDebug
	case model.LevelInfo:
		return model.LevelMaskInfo
	case model.LevelWarn:
		return model.LevelMaskWarn
	case model.LevelError:
		return model.LevelMaskError
	default:
		return model.LevelMaskAll
	}
}

func convertLevels(levels []uint8) []model.Level {
	out := make([]model.Level, len(levels))
	for i, level := range levels {
		out[i] = model.Level(level)
	}
	return out
}
