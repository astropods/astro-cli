package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/postman/astro/apps/astro-queen/internal/openmeter"
)

// ─── messages ─────────────────────────────────────────────────────────────────

type featuresLoadedMsg struct {
	rows [][]string
	ids  []string
	err  error
	at   time.Time
}

type featureDeletedMsg struct{ err error }
type featureCreatedMsg struct{ err error }

type featureDetailMsg struct {
	info featureInfo
	err  error
}

type featureMeterSlugsMsg struct {
	slugs []string
	err   error
}

// ─── data types ──────────────────────────────────────────────────────────────

type featureInfo struct {
	ID                   string
	Key                  string
	Name                 string
	MeterSlug            string
	MeterGroupByFilters  map[string]string
	AdvancedMeterGroupBy map[string]json.RawMessage
	Metadata             map[string]string
	CreatedAt            string
	UpdatedAt            string
	ArchivedAt           string
}

// ─── focus ───────────────────────────────────────────────────────────────────

type featureFocus int

const (
	featureFocusList featureFocus = iota
	featureFocusDetail
)

// ─── model ────────────────────────────────────────────────────────────────────

type featuresModel struct {
	client       *openmeter.Client
	t            tableModel
	ids          []string
	status       string
	width        int
	height       int
	focus        featureFocus
	showArchived bool

	// Detail
	detailID     string
	detailInfo   featureInfo
	detailLoaded bool

	// Create form (huh)
	huhForm     *huh.Form
	formFocused int
	formKey     string
	formName    string
	formMeter   string
	formFilter  string
	formMeta    string
}

func newFeaturesModel(client *openmeter.Client) *featuresModel {
	t := newTableModel([]string{"Key", "Name", "Meter Slug", "Group By Filters", "Archived", "Created"})
	t.SetFocused(true)

	return &featuresModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
	}
}

// ─── Tab interface ────────────────────────────────────────────────────────────

func (m *featuresModel) Name() string { return "Features" }

func (m *featuresModel) Init() tea.Cmd { return m.load() }

func (m *featuresModel) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case featuresLoadedMsg:
		if msg.err != nil {
			m.status = statusOK.Render("—")
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.t.SetRows(msg.rows)
		m.ids = msg.ids
		archived := ""
		if m.showArchived {
			archived = " (incl. archived)"
		}
		m.status = statusOK.Render(fmt.Sprintf(
			"%d features%s  —  %s", len(msg.rows), archived, msg.at.Format("15:04:05"),
		))
		return m, nil

	case featureDeletedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.focus = featureFocusList
		return m, m.load()

	case featureCreatedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		return m, m.load()

	case featureDetailMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return showErrMsg{msg.err.Error()} }
		}
		m.detailInfo = msg.info
		m.detailLoaded = true
		return m, nil

	case featureMeterSlugsMsg:
		// Rebuild the huh form with the meter options now available.
		if msg.err == nil && m.huhForm != nil {
			m.rebuildFormWithMeters(msg.slugs)
			return m, m.huhForm.Init()
		}
		return m, nil

	case tea.KeyMsg:
		switch m.focus {
		case featureFocusList:
			return m.updateList(msg)
		case featureFocusDetail:
			return m.updateDetail(msg)
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

// ─── list view ──────────────────────────────────────────────────────────────

func (m *featuresModel) updateList(msg tea.KeyMsg) (Tab, tea.Cmd) {
	if m.t.filtering {
		cmd := m.t.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "R":
		m.status = statusWIP.Render("Refreshing…")
		return m, m.load()

	case "A":
		m.showArchived = !m.showArchived
		m.status = statusWIP.Render("Refreshing…")
		return m, m.load()

	case "enter":
		idx := m.t.selectedRealIndex()
		if idx < 0 || idx >= len(m.ids) {
			return m, nil
		}
		id := m.ids[idx]
		m.focus = featureFocusDetail
		m.detailID = id
		m.detailLoaded = false
		return m, m.loadDetail(id)

	case "c":
		return m, tea.Batch(
			m.openCreateForm(),
			m.loadMeterSlugs(),
		)

	case "d":
		idx := m.t.selectedRealIndex()
		if idx < 0 || idx >= len(m.ids) {
			return m, nil
		}
		id := m.ids[idx]
		return m, func() tea.Msg {
			return showConfirmMsg{
				text: fmt.Sprintf("Archive feature %s? This cannot be undone.", statusWIP.Render(id)),
				fn: func() tea.Cmd {
					return func() tea.Msg {
						err := m.client.DeleteFeature(id)
						return featureDeletedMsg{err}
					}
				},
			}
		}
	}

	cmd := m.t.Update(msg)
	return m, cmd
}

// ─── detail view ────────────────────────────────────────────────────────────

func (m *featuresModel) updateDetail(msg tea.KeyMsg) (Tab, tea.Cmd) {
	switch msg.String() {
	case "backspace":
		m.focus = featureFocusList
		return m, nil
	case "d":
		id := m.detailID
		return m, func() tea.Msg {
			return showConfirmMsg{
				text: fmt.Sprintf("Archive feature %s? This cannot be undone.", statusWIP.Render(id)),
				fn: func() tea.Cmd {
					return func() tea.Msg {
						err := m.client.DeleteFeature(id)
						return featureDeletedMsg{err}
					}
				},
			}
		}
	}
	return m, nil
}

// ─── View ───────────────────────────────────────────────────────────────────

func (m *featuresModel) View(w, h int) string {
	if m.width == 0 {
		m.SetSize(w, h)
	}
	switch m.focus {
	case featureFocusDetail:
		return m.viewDetail()
	default:
		return m.t.View()
	}
}

func (m *featuresModel) viewDetail() string {
	if !m.detailLoaded {
		return statusWIP.Render("Loading feature…")
	}

	var b strings.Builder
	info := m.detailInfo
	kvStyle := lipgloss.NewStyle().Foreground(colDimmed)
	valStyle := lipgloss.NewStyle().Foreground(colFg)

	b.WriteString(titleStyle.Render(info.Key))
	if info.Name != "" && info.Name != info.Key {
		b.WriteString("  " + valStyle.Render(info.Name))
	}
	b.WriteString("\n")

	if info.ArchivedAt != "" {
		b.WriteString(statusErr.Render("ARCHIVED") + "  " + kvStyle.Render("at ") + valStyle.Render(info.ArchivedAt) + "\n")
	}

	b.WriteString(formSeparator(m.width) + "\n")

	b.WriteString(kvStyle.Render("ID:           ") + valStyle.Render(info.ID) + "\n")
	b.WriteString(kvStyle.Render("Key:          ") + valStyle.Render(info.Key) + "\n")
	b.WriteString(kvStyle.Render("Name:         ") + valStyle.Render(info.Name) + "\n")

	if info.MeterSlug != "" {
		b.WriteString(kvStyle.Render("Meter Slug:   ") + valStyle.Render(info.MeterSlug) + "\n")
	} else {
		b.WriteString(kvStyle.Render("Meter Slug:   ") + dimStyle.Render("(static — no meter)") + "\n")
	}

	if len(info.AdvancedMeterGroupBy) > 0 {
		b.WriteString(kvStyle.Render("Group By:     ") + "\n")
		for k, raw := range info.AdvancedMeterGroupBy {
			b.WriteString("  " + valStyle.Render(k) + kvStyle.Render(": ") + valStyle.Render(formatFilterString(raw)) + "\n")
		}
	} else if len(info.MeterGroupByFilters) > 0 {
		var parts []string
		for k, v := range info.MeterGroupByFilters {
			parts = append(parts, k+"="+v)
		}
		b.WriteString(kvStyle.Render("Group By:     ") + valStyle.Render(strings.Join(parts, ", ")) + "\n")
	}

	if len(info.Metadata) > 0 {
		var parts []string
		for k, v := range info.Metadata {
			parts = append(parts, k+"="+v)
		}
		b.WriteString(kvStyle.Render("Metadata:     ") + valStyle.Render(strings.Join(parts, ", ")) + "\n")
	}

	b.WriteString(kvStyle.Render("Created:      ") + valStyle.Render(info.CreatedAt) + "\n")
	b.WriteString(kvStyle.Render("Updated:      ") + valStyle.Render(info.UpdatedAt) + "\n")

	b.WriteString("\n")
	if info.ArchivedAt == "" {
		b.WriteString(descStyle.Render("Press d to archive this feature"))
	} else {
		b.WriteString(descStyle.Render("This feature is archived and cannot be modified"))
	}

	return b.String()
}

func (m *featuresModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.t.SetSize(w, h)
}

func (m *featuresModel) Tip() string {
	switch m.focus {
	case featureFocusDetail:
		return ""
	default:
		tip := "Features define what can be entitled to customers."
		if m.showArchived {
			tip += " (showing archived)"
		}
		return tip
	}
}

func (m *featuresModel) Status() string {
	switch m.focus {
	case featureFocusDetail:
		return statusOK.Render("feature: " + m.detailID)
	default:
		return m.status
	}
}

func (m *featuresModel) Hints(navMode bool) []KeyHint {
	if navMode {
		return []KeyHint{
			{"1-9/Tab", "switch tab"},
			{"q", "quit"},
			{"Esc", "back"},
		}
	}
	switch m.focus {
	case featureFocusDetail:
		hints := []KeyHint{
			{"Bksp", "back"},
		}
		if m.detailLoaded && m.detailInfo.ArchivedAt == "" {
			hints = append(hints, KeyHint{"d", "archive"})
		}
		return hints
	default:
		return []KeyHint{
			{"↑↓/jk", "navigate"},
			{"/", "search"},
			{"Enter", "detail"},
			{"c", "create"},
			{"d", "archive"},
			{"A", "toggle archived"},
			{"R", "refresh"},
			{"n", "nav mode"},
		}
	}
}

// ─── create form (huh) ─────────────────────────────────────────────────────

func (m *featuresModel) buildHuhForm(meterOpts []huh.Option[string]) *huh.Form {
	m.formKey = ""
	m.formName = ""
	m.formMeter = ""
	m.formFilter = ""
	m.formMeta = ""

	meterSelect := huh.NewSelect[string]().
		Key("meter").
		Title("Meter Slug").
		Description("Link to an existing meter. Leave as (none) for static features. Press / to search.").
		Height(12).
		Options(meterOpts...).
		Value(&m.formMeter)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("key").
				Title("Key *").
				Description("Unique identifier for this feature.").
				Placeholder("api_requests").
				Value(&m.formKey),

			huh.NewInput().
				Key("name").
				Title("Name *").
				Description("Human-readable display name.").
				Placeholder("API Requests").
				Value(&m.formName),

			meterSelect,

			huh.NewText().
				Key("filter").
				Title("Group By Filters").
				Description("One filter per line.").
				Placeholder("model=gpt-4\ntype=$in:input|output").
				Lines(4).
				CharLimit(500).
				Value(&m.formFilter),

			huh.NewInput().
				Key("meta").
				Title("Metadata").
				Description("Comma-separated key=value pairs.").
				Placeholder("env=prod,tier=enterprise").
				Value(&m.formMeta),

			huh.NewConfirm().
				Key("confirm").
				Title("Create this feature?").
				Description("Cannot be updated after creation.").
				Affirmative("Create").
				Negative("Cancel"),
		),
	).WithTheme(huhTheme).WithWidth(80)

	return form
}

func (m *featuresModel) openCreateForm() tea.Cmd {
	m.formFocused = 0
	// Build with a placeholder meter option; will be rebuilt when slugs arrive
	meterOpts := []huh.Option[string]{
		huh.NewOption("(none — static feature)", ""),
		huh.NewOption("Loading meters…", ""),
	}
	m.huhForm = m.buildHuhForm(meterOpts)
	initCmd := m.huhForm.Init()

	return tea.Batch(
		initCmd,
		func() tea.Msg {
			return showFormMsg{
				view:   m.viewHuhForm,
				update: m.updateHuhForm,
			}
		},
	)
}

func (m *featuresModel) rebuildFormWithMeters(slugs []string) {
	meterOpts := []huh.Option[string]{
		huh.NewOption("(none — static feature)", ""),
	}
	for _, s := range slugs {
		meterOpts = append(meterOpts, huh.NewOption(s, s))
	}
	m.huhForm = m.buildHuhForm(meterOpts)
}

// formFieldCount is the number of fields in the create form.
const formFieldCount = 6

func (m *featuresModel) viewHuhForm(width int) string {
	if m.huhForm == nil {
		return ""
	}

	rightW := 38
	borderW := 1
	leftW := width - rightW - borderW - 2
	if leftW < 40 {
		leftW = 40
		rightW = width - leftW - borderW - 2
	}
	if rightW < 10 {
		rightW = 0
	}

	m.huhForm = m.huhForm.WithWidth(leftW - 2)
	formView := m.huhForm.View()

	if rightW < 10 {
		return formView
	}

	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		PaddingRight(1).
		Render(formView)

	rightPane := lipgloss.NewStyle().
		Width(rightW).
		PaddingLeft(1).
		Render(m.featureFieldHelp())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m *featuresModel) updateHuhForm(msg tea.Msg) (bool, tea.Cmd) {
	if m.huhForm == nil {
		return true, nil
	}

	// Track field focus for the help pane.
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyTab:
			if m.formFocused < formFieldCount-1 {
				m.formFocused++
			}
		case tea.KeyShiftTab:
			if m.formFocused > 0 {
				m.formFocused--
			}
		case tea.KeyEnter:
			// Enter advances in input/select fields but not textarea (3) or confirm (5).
			if m.formFocused != 3 && m.formFocused != 5 && m.formFocused < formFieldCount-1 {
				m.formFocused++
			}
		}
	}

	form, cmd := m.huhForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.huhForm = f
	}

	if m.huhForm.State == huh.StateCompleted {
		// Check the confirm field — if user selected "Cancel", just close
		confirm := m.huhForm.GetBool("confirm")
		m.huhForm = nil
		if !confirm {
			return true, nil
		}
		return true, m.submitCreate()
	}

	if m.huhForm.State == huh.StateAborted {
		m.huhForm = nil
		return true, nil
	}

	return false, cmd
}

// featureFieldHelp returns contextual help text for the right pane
// based on which form field is currently focused.
func (m *featuresModel) featureFieldHelp() string {
	var title, body string
	switch m.formFocused {
	case 0:
		title = "Key (required)"
		body = "Unique identifier for the feature.\nLowercase alphanumeric + underscores.\nUsed to reference this feature in\nAPI calls and entitlements.\n\n  Example: api_requests\n  Example: token_usage"
	case 1:
		title = "Name (required)"
		body = "Human-readable display name shown\nin dashboards and entitlement UIs.\n\n  Example: API Requests\n  Example: Token Usage"
	case 2:
		title = "Meter Slug"
		body = "Link this feature to an existing\nmeter for usage-based tracking.\nLeave as (none) for static features\nthat don't track consumption.\n\nPress / to search through meters.\nUse ↑↓ to navigate options."
	case 3:
		title = "Group By Filters"
		body = "Filter meter data by group-by keys.\nOne filter per line.\n\nSimple equality:\n  model=gpt-4\n\nSet membership ($in):\n  type=$in:input|output\n\nNegation ($nin):\n  region=$nin:us-east|us-west"
	case 4:
		title = "Metadata"
		body = "Arbitrary key-value pairs attached\nto the feature. Comma-separated.\n\nUseful for tagging features by\nenvironment, tier, or team.\n\n  Example: env=prod,tier=enterprise"
	case 5:
		title = "Request Body"
		body = m.buildCreatePreview()
	}
	return helpTitleRender(title) + "\n\n" + helpBodyRender(body)
}

func (m *featuresModel) buildCreatePreview() string {
	body := map[string]any{
		"key":  strings.TrimSpace(m.formKey),
		"name": strings.TrimSpace(m.formName),
	}
	if m.formMeter != "" {
		body["meterSlug"] = m.formMeter
	}
	groupByStr := m.formFilter
	if groupByStr != "" {
		groupByStr = strings.ReplaceAll(groupByStr, "\n", ",")
		if gb := parseAdvancedGroupBy(groupByStr); len(gb) > 0 {
			body["advancedMeterGroupByFilters"] = gb
		}
	}
	if m.formMeta != "" {
		if md := parseKVPairs(m.formMeta); len(md) > 0 {
			body["metadata"] = md
		}
	}
	b, _ := json.MarshalIndent(body, "", "  ")
	return string(b)
}

func (m *featuresModel) submitCreate() tea.Cmd {
	key := strings.TrimSpace(m.formKey)
	name := strings.TrimSpace(m.formName)
	if key == "" || name == "" {
		missing := "key and name are"
		if key == "" && name != "" {
			missing = "key is"
		} else if key != "" && name == "" {
			missing = "name is"
		}
		return func() tea.Msg {
			return showErrMsg{missing + " required"}
		}
	}

	meterSlug := m.formMeter
	groupByStr := m.formFilter
	metadataStr := m.formMeta

	body := map[string]any{
		"key":  key,
		"name": name,
	}
	if meterSlug != "" {
		body["meterSlug"] = meterSlug
	}
	if groupByStr != "" {
		// Support both newline and comma separation
		groupByStr = strings.ReplaceAll(groupByStr, "\n", ",")
		gb := parseAdvancedGroupBy(groupByStr)
		if len(gb) > 0 {
			body["advancedMeterGroupByFilters"] = gb
		}
	}
	if metadataStr != "" {
		md := parseKVPairs(metadataStr)
		if len(md) > 0 {
			body["metadata"] = md
		}
	}

	client := m.client
	rawBody, _ := json.Marshal(body)

	return func() tea.Msg {
		_, err := client.CreateFeature(rawBody)
		return featureCreatedMsg{err}
	}
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *featuresModel) load() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListFeatures(m.showArchived)
		if err != nil {
			return featuresLoadedMsg{err: err, at: time.Now()}
		}

		features, err := parseFeaturesResponse(raw)
		if err != nil {
			return featuresLoadedMsg{err: err, at: time.Now()}
		}

		rows := make([][]string, len(features))
		ids := make([]string, len(features))
		for i, f := range features {
			gbStr := ""
			if len(f.AdvancedMeterGroupBy) > 0 {
				var parts []string
				for k, raw := range f.AdvancedMeterGroupBy {
					parts = append(parts, k+":"+formatFilterString(raw))
				}
				gbStr = strings.Join(parts, ", ")
			} else {
				for k, v := range f.MeterGroupByFilters {
					if gbStr != "" {
						gbStr += ", "
					}
					gbStr += k + "=" + v
				}
			}
			archived := ""
			if f.ArchivedAt != "" {
				archived = "yes"
			}
			rows[i] = []string{f.Key, f.Name, f.MeterSlug, gbStr, archived, f.CreatedAt}
			ids[i] = f.ID
		}
		return featuresLoadedMsg{rows: rows, ids: ids, at: time.Now()}
	}
}

type featureListItem struct {
	ID                   string                     `json:"id"`
	Key                  string                     `json:"key"`
	Name                 string                     `json:"name"`
	MeterSlug            string                     `json:"meterSlug"`
	MeterGroupByFilters  map[string]string          `json:"meterGroupByFilters"`
	AdvancedMeterGroupBy map[string]json.RawMessage `json:"advancedMeterGroupByFilters"`
	CreatedAt            string                     `json:"createdAt"`
	ArchivedAt           string                     `json:"archivedAt"`
}

func parseFeaturesResponse(raw json.RawMessage) ([]featureListItem, error) {
	var items []featureListItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	var paginated struct {
		Items []featureListItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &paginated); err != nil {
		return nil, fmt.Errorf("parse features: %w", err)
	}
	return paginated.Items, nil
}

func (m *featuresModel) loadDetail(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.GetFeature(id)
		if err != nil {
			return featureDetailMsg{err: err}
		}
		var f struct {
			ID                   string                     `json:"id"`
			Key                  string                     `json:"key"`
			Name                 string                     `json:"name"`
			MeterSlug            string                     `json:"meterSlug"`
			MeterGroupByFilters  map[string]string          `json:"meterGroupByFilters"`
			AdvancedMeterGroupBy map[string]json.RawMessage `json:"advancedMeterGroupByFilters"`
			Metadata             map[string]string          `json:"metadata"`
			CreatedAt            string                     `json:"createdAt"`
			UpdatedAt            string                     `json:"updatedAt"`
			ArchivedAt           string                     `json:"archivedAt"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return featureDetailMsg{err: fmt.Errorf("parse feature: %w", err)}
		}
		return featureDetailMsg{info: featureInfo{
			ID:                   f.ID,
			Key:                  f.Key,
			Name:                 f.Name,
			MeterSlug:            f.MeterSlug,
			MeterGroupByFilters:  f.MeterGroupByFilters,
			AdvancedMeterGroupBy: f.AdvancedMeterGroupBy,
			Metadata:             f.Metadata,
			CreatedAt:            f.CreatedAt,
			UpdatedAt:            f.UpdatedAt,
			ArchivedAt:           f.ArchivedAt,
		}}
	}
}

func (m *featuresModel) loadMeterSlugs() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListMeters()
		if err != nil {
			return featureMeterSlugsMsg{err: err}
		}
		var meters []struct {
			Slug string `json:"slug"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(raw, &meters); err != nil {
			return featureMeterSlugsMsg{err: fmt.Errorf("parse meters: %w", err)}
		}
		slugs := make([]string, len(meters))
		for i, meter := range meters {
			if meter.Slug != "" {
				slugs[i] = meter.Slug
			} else {
				slugs[i] = meter.ID
			}
		}
		return featureMeterSlugsMsg{slugs: slugs}
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func parseKVPairs(s string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			result[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return result
}

func parseAdvancedGroupBy(s string) map[string]any {
	result := make(map[string]any)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}

		if strings.HasPrefix(val, "$") {
			op, opVal, hasColon := strings.Cut(val, ":")
			if hasColon {
				switch op {
				case "$in", "$nin":
					items := strings.Split(opVal, "|")
					for i := range items {
						items[i] = strings.TrimSpace(items[i])
					}
					result[key] = map[string]any{op: items}
				default:
					result[key] = map[string]any{op: opVal}
				}
				continue
			}
		}

		result[key] = map[string]any{"$eq": val}
	}
	return result
}

func formatFilterString(raw json.RawMessage) string {
	var filter map[string]any
	if err := json.Unmarshal(raw, &filter); err != nil {
		return string(raw)
	}
	var parts []string
	for op, val := range filter {
		switch v := val.(type) {
		case string:
			parts = append(parts, op+" "+v)
		case []any:
			var items []string
			for _, item := range v {
				if s, ok := item.(string); ok {
					items = append(items, s)
				}
			}
			parts = append(parts, op+" ["+strings.Join(items, ", ")+"]")
		default:
			b, _ := json.Marshal(val)
			parts = append(parts, op+" "+string(b))
		}
	}
	return strings.Join(parts, ", ")
}
