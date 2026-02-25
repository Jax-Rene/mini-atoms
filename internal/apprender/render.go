package apprender

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	specpkg "mini-atoms/internal/spec"
)

type Mode string

const (
	ModeEditor        Mode = "editor"
	ModeShareReadonly Mode = "share_readonly"
)

func ParseMode(v string) Mode {
	switch Mode(strings.TrimSpace(v)) {
	case ModeShareReadonly:
		return ModeShareReadonly
	default:
		return ModeEditor
	}
}

func (m Mode) IsReadOnly() bool {
	return m == ModeShareReadonly
}

type Record struct {
	ID   int64
	Data map[string]any
}

type CollectionData struct {
	Schema  specpkg.CollectionSpec
	Records []Record
}

type RenderInput struct {
	Spec           specpkg.AppSpec
	Mode           Mode
	SelectedPageID string
	Collections    map[string]CollectionData
}

type AppView struct {
	AppName     string
	Mode        string
	ReadOnly    bool
	Pages       []PageTabView
	CurrentPage PageView
}

type PageTabView struct {
	ID     string
	Title  string
	Active bool
}

type PageView struct {
	ID     string
	Title  string
	Blocks []BlockView
}

type BlockView struct {
	Type            string
	Collection      string
	Label           string
	Nav             *NavBlockView
	Form            *FormBlockView
	List            *ListBlockView
	Toggle          *ToggleBlockView
	Stats           *StatsBlockView
	Timer           *TimerBlockView
	UnsupportedText string
}

type NavBlockView struct {
	Items []NavItemView
}

type NavItemView struct {
	Label  string
	PageID string
	Active bool
}

type FormBlockView struct {
	Collection string
	Fields     []FormFieldView
}

type FormFieldView struct {
	Name      string
	Label     string
	Type      string
	InputType string
	Required  bool
	Step      string
	Options   []string
}

type ListBlockView struct {
	Collection string
	EditPageID string
	Columns    []ListColumnView
	Rows       []ListRowView
}

type ListColumnView struct {
	Name  string
	Label string
}

type ListRowView struct {
	ID    int64
	Cells []ListCellView
}

type ListCellView struct {
	FieldName  string
	ValueText  string
	ValueClass string
}

type ToggleBlockView struct {
	Collection string
	Field      string
	Rows       []ToggleRowView
}

type ToggleRowView struct {
	RecordID int64
	Title    string
	On       bool
}

type StatsBlockView struct {
	Label string
	Value string
}

type TimerBlockView struct {
	SessionCollection string
	WorkMinutes       int
	BreakMinutes      int

	TaskField        string
	MinutesField     string
	CompletedAtField string
	CanSaveSession   bool
}

func RenderApp(input RenderInput) (AppView, error) {
	if err := specpkg.ValidateAppSpec(input.Spec); err != nil {
		return AppView{}, fmt.Errorf("render app: invalid spec: %w", err)
	}

	selected := strings.TrimSpace(input.SelectedPageID)
	currentPage := input.Spec.Pages[0]
	if selected != "" {
		for _, p := range input.Spec.Pages {
			if p.ID == selected {
				currentPage = p
				break
			}
		}
	}

	pageTitles := make(map[string]string, len(input.Spec.Pages))
	pages := make([]PageTabView, 0, len(input.Spec.Pages))
	for _, p := range input.Spec.Pages {
		title := pageTitle(p)
		pageTitles[p.ID] = title
		pages = append(pages, PageTabView{
			ID:     p.ID,
			Title:  title,
			Active: p.ID == currentPage.ID,
		})
	}

	blocks := make([]BlockView, 0, len(currentPage.Blocks))
	for _, b := range currentPage.Blocks {
		blockView := BlockView{
			Type:       b.Type,
			Collection: b.Collection,
			Label:      strings.TrimSpace(b.Label),
		}

		switch b.Type {
		case "nav":
			navItems := make([]NavItemView, 0, len(b.Items))
			for _, item := range b.Items {
				label := strings.TrimSpace(item.Label)
				if label == "" {
					label = pageTitles[item.PageID]
				}
				if label == "" {
					label = item.PageID
				}
				navItems = append(navItems, NavItemView{
					Label:  label,
					PageID: item.PageID,
					Active: item.PageID == currentPage.ID,
				})
			}
			blockView.Nav = &NavBlockView{Items: navItems}

		case "form":
			coll, ok := input.Collections[b.Collection]
			if !ok {
				return AppView{}, fmt.Errorf("render app: missing collection data %q", b.Collection)
			}
			fields := resolveFieldsForBlock(b.Fields, coll.Schema)
			formFields := make([]FormFieldView, 0, len(fields))
			for _, f := range fields {
				formFields = append(formFields, FormFieldView{
					Name:      f.Name,
					Label:     humanizeName(f.Name),
					Type:      f.Type,
					InputType: formInputType(f.Type),
					Required:  f.Required,
					Step:      formInputStep(f.Type),
					Options:   append([]string(nil), f.Options...),
				})
			}
			blockView.Form = &FormBlockView{
				Collection: b.Collection,
				Fields:     formFields,
			}

		case "list":
			coll, ok := input.Collections[b.Collection]
			if !ok {
				return AppView{}, fmt.Errorf("render app: missing collection data %q", b.Collection)
			}
			fields := resolveFieldsForBlock(b.Fields, coll.Schema)
			columns := make([]ListColumnView, 0, len(fields))
			for _, f := range fields {
				columns = append(columns, ListColumnView{Name: f.Name, Label: humanizeName(f.Name)})
			}
			rows := make([]ListRowView, 0, len(coll.Records))
			for _, rec := range coll.Records {
				row := ListRowView{ID: rec.ID, Cells: make([]ListCellView, 0, len(fields))}
				for _, f := range fields {
					val := rec.Data[f.Name]
					row.Cells = append(row.Cells, ListCellView{
						FieldName:  f.Name,
						ValueText:  formatFieldValue(f.Type, val),
						ValueClass: valueClassForField(f.Type, val),
					})
				}
				rows = append(rows, row)
			}
			blockView.List = &ListBlockView{
				Collection: b.Collection,
				EditPageID: findPreferredFormPageID(input.Spec, currentPage.ID, b.Collection),
				Columns:    columns,
				Rows:       rows,
			}

		case "toggle":
			coll, ok := input.Collections[b.Collection]
			if !ok {
				return AppView{}, fmt.Errorf("render app: missing collection data %q", b.Collection)
			}
			titleField := pickTitleField(coll.Schema)
			rows := make([]ToggleRowView, 0, len(coll.Records))
			for _, rec := range coll.Records {
				on, _ := recordBool(rec.Data[b.Field])
				title := formatFieldValue(titleField.Type, rec.Data[titleField.Name])
				if strings.TrimSpace(title) == "" || title == "—" {
					title = fmt.Sprintf("Record #%d", rec.ID)
				}
				rows = append(rows, ToggleRowView{
					RecordID: rec.ID,
					Title:    title,
					On:       on,
				})
			}
			blockView.Toggle = &ToggleBlockView{
				Collection: b.Collection,
				Field:      b.Field,
				Rows:       rows,
			}

		case "stats":
			coll, ok := input.Collections[b.Collection]
			if !ok {
				return AppView{}, fmt.Errorf("render app: missing collection data %q", b.Collection)
			}
			value, err := ComputeStatValue(b, coll.Schema, coll.Records)
			if err != nil {
				return AppView{}, fmt.Errorf("render app: %w", err)
			}
			label := strings.TrimSpace(b.Label)
			if label == "" {
				switch b.Metric {
				case "count":
					label = "Count"
				case "sum":
					label = "Sum"
				default:
					label = "Stats"
				}
			}
			blockView.Stats = &StatsBlockView{Label: label, Value: value}

		case "timer":
			timerView := &TimerBlockView{
				SessionCollection: strings.TrimSpace(b.SessionCollection),
				WorkMinutes:       b.WorkMinutes,
				BreakMinutes:      b.BreakMinutes,
			}
			if timerView.WorkMinutes <= 0 {
				timerView.WorkMinutes = 25
			}
			if timerView.BreakMinutes <= 0 {
				timerView.BreakMinutes = 5
			}
			if timerView.SessionCollection != "" {
				if coll, ok := input.Collections[timerView.SessionCollection]; ok {
					timerView.TaskField = pickTimerTaskField(coll.Schema)
					timerView.MinutesField = pickTimerMinutesField(coll.Schema)
					timerView.CompletedAtField = pickTimerCompletedAtField(coll.Schema)
					timerView.CanSaveSession = timerView.MinutesField != ""
				}
			}
			blockView.Timer = timerView

		default:
			blockView.UnsupportedText = fmt.Sprintf("block type %q 暂未在 M5 渲染器中实现", b.Type)
		}

		blocks = append(blocks, blockView)
	}

	return AppView{
		AppName:  strings.TrimSpace(input.Spec.AppName),
		Mode:     string(input.Mode),
		ReadOnly: input.Mode.IsReadOnly(),
		Pages:    pages,
		CurrentPage: PageView{
			ID:     currentPage.ID,
			Title:  pageTitle(currentPage),
			Blocks: blocks,
		},
	}, nil
}

func findPreferredFormPageID(appSpec specpkg.AppSpec, currentPageID, collectionName string) string {
	current := strings.TrimSpace(currentPageID)
	collection := strings.TrimSpace(collectionName)
	if collection == "" {
		return current
	}

	firstMatch := ""
	for _, page := range appSpec.Pages {
		pageID := strings.TrimSpace(page.ID)
		if pageID == "" {
			continue
		}
		for _, block := range page.Blocks {
			if block.Type != "form" {
				continue
			}
			if strings.TrimSpace(block.Collection) != collection {
				continue
			}
			if pageID == current {
				return pageID
			}
			if firstMatch == "" {
				firstMatch = pageID
			}
			break
		}
	}

	if firstMatch != "" {
		return firstMatch
	}
	return current
}

func ComputeStatValue(block specpkg.BlockSpec, schema specpkg.CollectionSpec, records []Record) (string, error) {
	switch block.Metric {
	case "count":
		return strconv.Itoa(len(records)), nil
	case "sum":
		field, ok := findSchemaField(schema, block.Field)
		if !ok {
			return "", fmt.Errorf("stats sum field %q not found in collection %q", block.Field, schema.Name)
		}
		var total float64
		for _, rec := range records {
			n, ok := numberValue(rec.Data[field.Name])
			if !ok {
				continue
			}
			total += n
		}
		if field.Type == specpkg.FieldTypeInt && math.Abs(total-math.Round(total)) < 1e-9 {
			return strconv.FormatInt(int64(math.Round(total)), 10), nil
		}
		return strconv.FormatFloat(total, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("unsupported stats metric %q", block.Metric)
	}
}

func resolveFieldsForBlock(fieldNames []string, schema specpkg.CollectionSpec) []specpkg.FieldSpec {
	if len(fieldNames) == 0 {
		out := make([]specpkg.FieldSpec, 0, len(schema.Fields))
		out = append(out, schema.Fields...)
		sort.SliceStable(out, func(i, j int) bool {
			wi := defaultFieldSortWeight(out[i])
			wj := defaultFieldSortWeight(out[j])
			if wi != wj {
				return wi < wj
			}
			return false
		})
		return out
	}
	out := make([]specpkg.FieldSpec, 0, len(fieldNames))
	for _, name := range fieldNames {
		if f, ok := findSchemaField(schema, name); ok {
			out = append(out, f)
		}
	}
	return out
}

func findSchemaField(schema specpkg.CollectionSpec, name string) (specpkg.FieldSpec, bool) {
	for _, f := range schema.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return specpkg.FieldSpec{}, false
}

func pickTitleField(schema specpkg.CollectionSpec) specpkg.FieldSpec {
	for _, f := range schema.Fields {
		if f.Type == specpkg.FieldTypeText {
			return f
		}
	}
	if len(schema.Fields) > 0 {
		return schema.Fields[0]
	}
	return specpkg.FieldSpec{Name: "id", Type: specpkg.FieldTypeText}
}

func pageTitle(p specpkg.PageSpec) string {
	if strings.TrimSpace(p.Title) != "" {
		return p.Title
	}
	return humanizeName(p.ID)
}

func humanizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	parts := strings.Fields(s)
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func formInputType(fieldType string) string {
	switch fieldType {
	case specpkg.FieldTypeInt, specpkg.FieldTypeReal:
		return "number"
	case specpkg.FieldTypeBool:
		return "checkbox"
	case specpkg.FieldTypeDate:
		return "date"
	case specpkg.FieldTypeDatetime:
		return "datetime-local"
	default:
		return "text"
	}
}

func formInputStep(fieldType string) string {
	switch fieldType {
	case specpkg.FieldTypeInt:
		return "1"
	case specpkg.FieldTypeReal:
		return "any"
	default:
		return ""
	}
}

func formatFieldValue(fieldType string, v any) string {
	if v == nil {
		return "—"
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return "—"
	}
	switch fieldType {
	case specpkg.FieldTypeBool:
		b, ok := recordBool(v)
		if !ok {
			return "—"
		}
		if b {
			return "已完成"
		}
		return "未完成"
	case specpkg.FieldTypeInt:
		if n, ok := numberValue(v); ok {
			if math.Abs(n-math.Round(n)) < 1e-9 {
				return strconv.FormatInt(int64(math.Round(n)), 10)
			}
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
	case specpkg.FieldTypeReal:
		if n, ok := numberValue(v); ok {
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
	case specpkg.FieldTypeDate:
		if s, ok := v.(string); ok {
			return formatDateValue(s)
		}
	case specpkg.FieldTypeDatetime:
		if s, ok := v.(string); ok {
			return formatDateTimeValue(s)
		}
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func valueClassForField(fieldType string, v any) string {
	if fieldType == specpkg.FieldTypeBool {
		if b, ok := recordBool(v); ok && b {
			return "m5-value-bool-on"
		}
		return "m5-value-bool-off"
	}
	return ""
}

func numberValue(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case jsonNumberLike:
		n, err := strconv.ParseFloat(string(x), 64)
		return n, err == nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

type jsonNumberLike string

func recordBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "on", "yes":
			return true, true
		case "0", "false", "off", "no":
			return false, true
		}
	case float64:
		return x != 0, true
	case int64:
		return x != 0, true
	}
	return false, false
}

func defaultFieldSortWeight(f specpkg.FieldSpec) int {
	switch f.Type {
	case specpkg.FieldTypeText:
		if strings.EqualFold(f.Name, "title") || strings.EqualFold(f.Name, "name") || strings.Contains(strings.ToLower(f.Name), "title") {
			return 0
		}
		return 1
	case specpkg.FieldTypeEnum:
		return 2
	case specpkg.FieldTypeDate, specpkg.FieldTypeDatetime:
		return 3
	case specpkg.FieldTypeInt, specpkg.FieldTypeReal:
		return 4
	case specpkg.FieldTypeBool:
		return 5
	default:
		return 6
	}
}

func pickTimerTaskField(schema specpkg.CollectionSpec) string {
	for _, candidate := range []string{"task", "title", "name"} {
		if f, ok := findSchemaField(schema, candidate); ok && f.Type == specpkg.FieldTypeText {
			return f.Name
		}
	}
	for _, f := range schema.Fields {
		if f.Type == specpkg.FieldTypeText {
			return f.Name
		}
	}
	return ""
}

func pickTimerMinutesField(schema specpkg.CollectionSpec) string {
	for _, candidate := range []string{"minutes", "duration_minutes", "duration", "work_minutes"} {
		if f, ok := findSchemaField(schema, candidate); ok && (f.Type == specpkg.FieldTypeInt || f.Type == specpkg.FieldTypeReal) {
			return f.Name
		}
	}
	for _, f := range schema.Fields {
		if f.Type == specpkg.FieldTypeInt || f.Type == specpkg.FieldTypeReal {
			return f.Name
		}
	}
	return ""
}

func pickTimerCompletedAtField(schema specpkg.CollectionSpec) string {
	for _, candidate := range []string{"completed_at", "ended_at", "finished_at", "created_at"} {
		if f, ok := findSchemaField(schema, candidate); ok && (f.Type == specpkg.FieldTypeDatetime || f.Type == specpkg.FieldTypeDate) {
			return f.Name
		}
	}
	for _, f := range schema.Fields {
		if f.Type == specpkg.FieldTypeDatetime || f.Type == specpkg.FieldTypeDate {
			return f.Name
		}
	}
	return ""
}

func formatDateValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

func formatDateTimeValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
	}
	return s
}
