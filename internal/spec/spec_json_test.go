package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAppSpecJSON_UnmarshalBlockFieldsObjectArray(t *testing.T) {
	t.Parallel()

	raw := `{
		"app_name":"Todo App",
		"collections":[
			{"name":"todos","fields":[
				{"name":"title","type":"text"},
				{"name":"done","type":"bool"}
			]}
		],
		"pages":[
			{"id":"home","blocks":[
				{"type":"list","collection":"todos","fields":[
					{"name":"title","label":"标题"},
					{"field":"done","label":"完成"}
				]}
			]}
		]
	}`

	var got AppSpec
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(got.Pages) != 1 || len(got.Pages[0].Blocks) != 1 {
		t.Fatalf("unexpected pages/blocks: %#v", got.Pages)
	}

	wantFields := []string{"title", "done"}
	if !reflect.DeepEqual([]string(got.Pages[0].Blocks[0].Fields), wantFields) {
		t.Fatalf("block.fields = %#v, want %#v", got.Pages[0].Blocks[0].Fields, wantFields)
	}

	if err := ValidateAppSpec(got); err != nil {
		t.Fatalf("ValidateAppSpec() error = %v", err)
	}
}
