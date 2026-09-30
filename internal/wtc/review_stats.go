package wtc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// collectReviewStats keeps the launcher-owned usage record when one exists.
// Built-in agents report usage in their output; their parsers are best effort.
func collectReviewStats(path, agent, model string, elapsed time.Duration, status string, output []byte) reviewRunStats {
	stats := reviewRunStats{Agent: agent, Model: model, Seconds: elapsed.Seconds(), Status: status}
	if os.Getenv("WTC_REVIEW_AGENT_CMD") != "" {
		var custom reviewRunStats
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &custom) == nil {
			stats.InputTokens, stats.OutputTokens = custom.InputTokens, custom.OutputTokens
			stats.CacheReadTokens, stats.CacheWriteTokens = custom.CacheReadTokens, custom.CacheWriteTokens
			stats.CostUSD, stats.Turns = custom.CostUSD, custom.Turns
			if model == "" && custom.Model != "" {
				stats.Model = custom.Model
			}
			if custom.Seconds > 0 {
				stats.Seconds = custom.Seconds
			}
		}
		return stats
	}
	objects := reviewJSONObjects(output)
	if agent == "codex" {
		var in, out, read, write, turns int64
		for _, object := range objects {
			if object["type"] != "turn.completed" {
				continue
			}
			usage := reviewMap(object["usage"])
			cached := reviewInt(usage["cached_input_tokens"])
			in += max(0, reviewInt(usage["input_tokens"])-cached)
			out += reviewInt(usage["output_tokens"])
			read += cached
			write += reviewInt(usage["cache_write_input_tokens"])
			turns++
		}
		if turns > 0 {
			stats.InputTokens, stats.OutputTokens = &in, &out
			stats.CacheReadTokens, stats.CacheWriteTokens = &read, &write
			stats.Turns = &turns
		}
		return stats
	}
	if agent != "claude" && agent != "grok" {
		return stats
	}
	var result map[string]any
	for _, object := range objects {
		if agent == "grok" && (object["text"] != nil || object["usage"] != nil) || agent == "claude" && (object["type"] == "result" || object["usage"] != nil) {
			result = object
		}
	}
	if result == nil {
		return stats
	}
	usage := reviewMap(result["usage"])
	stats.InputTokens = reviewIntPtr(usage["input_tokens"])
	stats.OutputTokens = reviewIntPtr(usage["output_tokens"])
	stats.CacheReadTokens = reviewIntPtr(usage["cache_read_input_tokens"])
	stats.CacheWriteTokens = reviewIntPtr(usage["cache_creation_input_tokens"])
	stats.Turns = reviewIntPtr(result["num_turns"])
	if cost, ok := result["total_cost_usd"].(float64); ok && !math.IsNaN(cost) && !math.IsInf(cost, 0) {
		stats.CostUSD = &cost
	}
	if model == "" {
		var best float64 = -1
		for name, value := range reviewMap(result["modelUsage"]) {
			count, _ := reviewMap(value)["outputTokens"].(float64)
			if count > best || count == best && name < stats.Model {
				best, stats.Model = count, name
			}
		}
	}
	return stats
}

func reviewJSONObjects(output []byte) []map[string]any {
	var objects []map[string]any
	var whole map[string]any
	if json.Unmarshal(output, &whole) == nil {
		return []map[string]any{whole}
	}
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		var object map[string]any
		if json.Unmarshal(line, &object) == nil {
			objects = append(objects, object)
		}
	}
	return objects
}

func reviewMap(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return nil
}

func reviewInt(value any) int64 {
	if number, ok := value.(float64); ok && number >= 0 && number <= math.MaxInt64 {
		return int64(number)
	}
	return 0
}

func reviewIntPtr(value any) *int64 {
	if _, ok := value.(float64); !ok {
		return nil
	}
	count := reviewInt(value)
	return &count
}

func reviewStatsTable(bundle string, wall time.Duration) (string, error) {
	paths, err := filepath.Glob(filepath.Join(bundle, "stats", "*.json"))
	if err != nil {
		return "", err
	}
	sort.Slice(paths, func(i, j int) bool {
		a, b := filepath.Base(paths[i]), filepath.Base(paths[j])
		if a == "lead.json" {
			return false
		}
		if b == "lead.json" {
			return true
		}
		return a < b
	})
	lines := []string{"### Run stats", "", "| Concern | Agent | Time | Tokens in / out (cache read) | Cost |", "|---|---|---|---|---|"}
	var seconds float64
	var in, out, read int64
	var cost float64
	knownCost, unknownCost := false, false
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		var stats reviewRunStats
		if err := json.Unmarshal(data, &stats); err != nil {
			return "", err
		}
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		if stats.Status == "skipped" {
			lines = append(lines, fmt.Sprintf("| %s | - | - | - | - |", id))
			continue
		}
		seconds += stats.Seconds
		in += reviewValue(stats.InputTokens)
		out += reviewValue(stats.OutputTokens)
		read += reviewValue(stats.CacheReadTokens)
		if stats.CostUSD == nil {
			unknownCost = true
		} else {
			knownCost = true
			cost += *stats.CostUSD
		}
		tokens := "-"
		if stats.InputTokens != nil || stats.OutputTokens != nil {
			tokens = fmt.Sprintf("%s / %s (%s)", reviewTokens(stats.InputTokens), reviewTokens(stats.OutputTokens), reviewTokens(stats.CacheReadTokens))
		}
		name := id
		if stats.Status != "ok" && stats.Status != "" {
			name += " (" + stats.Status + ")"
		}
		lines = append(lines, fmt.Sprintf("| %s | %s:%s | %s | %s | %s |", name, stats.Agent, stats.Model, reviewSeconds(stats.Seconds), tokens, reviewCost(stats.CostUSD)))
	}
	totalCost := "-"
	if knownCost {
		totalCost = fmt.Sprintf("$%.2f", cost)
		if unknownCost {
			totalCost += "+"
		}
	}
	lines = append(lines, fmt.Sprintf("| **Total** | | %s | %s / %s (%s) | %s |", reviewSeconds(seconds), reviewTokens(&in), reviewTokens(&out), reviewTokens(&read), totalCost), "", fmt.Sprintf("Wall-clock for the whole run: %s (agent time summed: %s).", reviewSeconds(wall.Seconds()), reviewSeconds(seconds)))
	return strings.Join(lines, "\n"), nil
}

func reviewValue(value *int64) int64 {
	if value != nil {
		return *value
	}
	return 0
}
func reviewSeconds(value float64) string {
	seconds := int64(value)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%02dm", seconds/3600, seconds%3600/60)
}
func reviewTokens(value *int64) string {
	if value == nil {
		return "-"
	}
	n := *value
	if n < 1000 {
		return fmt.Sprint(n)
	}
	if n < 10000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	if n < 1000000 {
		return fmt.Sprintf("%dk", int64(math.Round(float64(n)/1000)))
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}
func reviewCost(value *float64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", *value)
}
