package spec

type AppSpec struct {
	AppName     string           `json:"app_name"`
	Theme       string           `json:"theme,omitempty"`
	Collections []CollectionSpec `json:"collections"`
	Pages       []PageSpec       `json:"pages"`
}

type CollectionSpec struct {
	Name   string      `json:"name"`
	Fields []FieldSpec `json:"fields"`
}

type FieldSpec struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
}

type PageSpec struct {
	ID     string      `json:"id"`
	Title  string      `json:"title,omitempty"`
	Blocks []BlockSpec `json:"blocks"`
}

type NavItemSpec struct {
	Label  string `json:"label"`
	PageID string `json:"page_id"`
}

type BlockFieldList []string

type BlockSpec struct {
	Type string `json:"type"`

	Collection string         `json:"collection,omitempty"`
	Fields     BlockFieldList `json:"fields,omitempty"`
	Field      string         `json:"field,omitempty"`

	Metric string `json:"metric,omitempty"`
	Label  string `json:"label,omitempty"`

	Items []NavItemSpec `json:"items,omitempty"`

	SessionCollection string `json:"session_collection,omitempty"`
	WorkMinutes       int    `json:"work_minutes,omitempty"`
	BreakMinutes      int    `json:"break_minutes,omitempty"`
}

const (
	FieldTypeText     = "text"
	FieldTypeInt      = "int"
	FieldTypeReal     = "real"
	FieldTypeBool     = "bool"
	FieldTypeDate     = "date"
	FieldTypeDatetime = "datetime"
	FieldTypeEnum     = "enum"
)

var AllowedFieldTypes = map[string]struct{}{
	FieldTypeText:     {},
	FieldTypeInt:      {},
	FieldTypeReal:     {},
	FieldTypeBool:     {},
	FieldTypeDate:     {},
	FieldTypeDatetime: {},
	FieldTypeEnum:     {},
}

var AllowedBlockTypes = map[string]struct{}{
	"nav":    {},
	"list":   {},
	"form":   {},
	"toggle": {},
	"stats":  {},
	"timer":  {},
}
