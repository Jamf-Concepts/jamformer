// Copyright 2026, Jamf Software LLC

package protect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// listResourceEvent matches the relevant fields of the
// "list_resource_found" event emitted by `terraform query -json`.
type listResourceEvent struct {
	Type             string `json:"type"`
	ListResourceFound struct {
		Address      string `json:"address"`
		DisplayName  string `json:"display_name"`
		ResourceType string `json:"resource_type"`
		Identity     struct {
			ID string `json:"id"`
		} `json:"identity"`
	} `json:"list_resource_found"`
}

// ParseQueryEvents reads a terraform query -json event log and returns a
// nested map of resourceType -> id -> display_name. Events without an ID or
// display_name are skipped. Lines that fail to parse are ignored (the log
// includes other event types we don't care about).
func ParseQueryEvents(eventsFile string) (map[string]map[string]string, error) {
	f, err := os.Open(eventsFile)
	if err != nil {
		return nil, fmt.Errorf("opening events file: %w", err)
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]map[string]string)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var ev listResourceEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Type != "list_resource_found" {
			continue
		}
		id := ev.ListResourceFound.Identity.ID
		name := ev.ListResourceFound.DisplayName
		rt := ev.ListResourceFound.ResourceType
		if id == "" || name == "" || rt == "" {
			continue
		}
		if out[rt] == nil {
			out[rt] = make(map[string]string)
		}
		out[rt][id] = name
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning events: %w", err)
	}
	return out, nil
}
