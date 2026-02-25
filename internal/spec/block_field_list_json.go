package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func (l *BlockFieldList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*l = nil
		return nil
	}

	var names []string
	if err := json.Unmarshal(data, &names); err == nil {
		*l = BlockFieldList(trimNonEmptyStrings(names))
		return nil
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(data, &rawItems); err != nil {
		return err
	}

	out := make([]string, 0, len(rawItems))
	for i, item := range rawItems {
		item = bytes.TrimSpace(item)
		if len(item) == 0 || bytes.Equal(item, []byte("null")) {
			continue
		}

		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
			continue
		}

		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			return fmt.Errorf("block.fields[%d]: expected string or object: %w", i, err)
		}

		name, ok, err := readStringKey(obj, "name", "field", "id", "key")
		if err != nil {
			return fmt.Errorf("block.fields[%d]: %w", i, err)
		}
		if !ok {
			return fmt.Errorf("block.fields[%d]: object must include one string key: name/field/id/key", i)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("block.fields[%d]: field name is empty", i)
		}
		out = append(out, name)
	}

	*l = BlockFieldList(out)
	return nil
}

func trimNonEmptyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func readStringKey(obj map[string]json.RawMessage, keys ...string) (string, bool, error) {
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", false, fmt.Errorf("%s must be string", key)
		}
		return v, true, nil
	}
	return "", false, nil
}
