package openmeter_tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

// ─── data types ──────────────────────────────────────────────────────────────

type featureInfo struct {
	ID                 string
	Key                string
	Name               string
	MeterSlug          string
	MeterGroupByFilter map[string]string
	Metadata           map[string]string
	CreatedAt          string
	UpdatedAt          string
	ArchivedAt         string
}

// ─── focus ───────────────────────────────────────────────────────────────────

type featureFocus int

const (
	featureFocusList featureFocus = iota
	featureFocusDetail
)

// ─── create form field indices ──────────────────────────────────────────────

const (
	featFieldKey = iota
	featFieldName
	featFieldMeterSlug
	featFieldGroupByFilters
	featFieldMetadata
	featFieldCount
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

	// Create form
	createFields  [5]textinput.Model
	createFocused int
	createStatus  string
}

func newFeaturesModel(client *openmeter.Client) *featuresModel {
	t := newTableModel([]string{"Key", "Name", "Meter Slug", "Group By Filters", "Archived", "Created"})
	t.SetFocused(true)

	mkInput := func(placeholder string, charLimit int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		ti.Prompt = ""
		return ti
	}

	return &featuresModel{
		client: client,
		t:      t,
		status: statusWIP.Render("Loading…"),
		createFields: [5]textinput.Model{
			mkInput("api_requests", 64),
			mkInput("API Requests", 256),
			mkInput("tokens_total", 64),
			mkInput("model=gpt-4,type=input", 500),
			mkInput("key=value,env=prod", 500),
		},
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
		m.createFocused = featFieldKey
		m.createStatus = ""
		for i := range m.createFields {
			m.createFields[i].Reset()
			m.createFields[i].Blur()
		}
		m.createFields[0].Focus()
		return m, func() tea.Msg {
			return showFormMsg{
				view:   m.viewCreateForm,
				update: m.updateCreateForm,
			}
		}

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
		archivedHint := ""
		if m.showArchived {
			archivedHint = " (showing archived)"
		}
		tip := tipStyle.Render("Features define what can be entitled to customers." + archivedHint)
		return tip + "\n" + m.t.View()
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

	// Header
	b.WriteString(titleStyle.Render(info.Key))
	if info.Name != "" && info.Name != info.Key {
		b.WriteString("  " + valStyle.Render(info.Name))
	}
	b.WriteString("\n")

	// Archived badge
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

	if len(info.MeterGroupByFilter) > 0 {
		var parts []string
		for k, v := range info.MeterGroupByFilter {
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
			{"Esc", "nav mode"},
		}
	}
}

// ─── create form ────────────────────────────────────────────────────────────

func (m *featuresModel) updateCreateForm(msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		return true, nil
	case "tab", "down":
		next := (m.createFocused + 1) % featFieldCount
		m.createFields[m.createFocused].Blur()
		m.createFocused = next
		m.createFields[next].Focus()
		return false, textinput.Blink
	case "shift+tab", "up":
		next := (m.createFocused + featFieldCount - 1) % featFieldCount
		m.createFields[m.createFocused].Blur()
		m.createFocused = next
		m.createFields[next].Focus()
		return false, textinput.Blink
	case "enter":
		cmd := m.submitCreate()
		if cmd == nil {
			return false, nil
		}
		return true, cmd
	}

	var cmd tea.Cmd
	m.createFields[m.createFocused], cmd = m.createFields[m.createFocused].Update(msg)
	return false, cmd
}

func (m *featuresModel) submitCreate() tea.Cmd {
	key := m.createFields[featFieldKey].Value()
	name := m.createFields[featFieldName].Value()
	meterSlug := m.createFields[featFieldMeterSlug].Value()
	groupByStr := m.createFields[featFieldGroupByFilters].Value()
	metadataStr := m.createFields[featFieldMetadata].Value()

	if key == "" {
		m.createStatus = statusErr.Render("Key is required")
		return nil
	}
	if name == "" {
		m.createStatus = statusErr.Render("Name is required")
		return nil
	}

	body := map[string]any{
		"key":  key,
		"name": name,
	}
	if meterSlug != "" {
		body["meterSlug"] = meterSlug
	}
	if groupByStr != "" {
		gb := parseKVPairs(groupByStr)
		if len(gb) > 0 {
			body["meterGroupByFilters"] = gb
		}
	}
	if metadataStr != "" {
		md := parseKVPairs(metadataStr)
		if len(md) > 0 {
			body["metadata"] = md
		}
	}

	m.createStatus = statusWIP.Render("Creating…")
	return func() tea.Msg {
		raw, _ := json.Marshal(body)
		_, err := m.client.CreateFeature(raw)
		return featureCreatedMsg{err}
	}
}

func (m *featuresModel) createFieldHelp() (string, string) {
	switch m.createFocused {
	case featFieldKey:
		return "Key (required)",
			"Unique identifier for the feature.\nLowercase alphanumeric + underscores.\nPattern: ^[a-z0-9]+(?:_[a-z0-9]+)*$\n\n  Example: api_requests\n  Example: tokens_total"
	case featFieldName:
		return "Name (required)",
			"Human-readable name.\n\n  Example: API Requests\n  Example: Tokens Total"
	case featFieldMeterSlug:
		return "Meter Slug",
			"Link to an existing meter.\nLeave blank for a static feature.\n\nMetered features track usage against\na meter. Supported aggregations:\nSUM, COUNT, UNIQUE_COUNT, LATEST.\n\n  Example: tokens_total"
	case featFieldGroupByFilters:
		return "Meter Group By Filters",
			"Filter meter data by groupBy fields.\nFormat: key=value,key2=value2\n\nUseful when the meter scope is\nbroader than the feature.\n\n  Example: model=gpt-4,type=input"
	case featFieldMetadata:
		return "Metadata",
			"Key-value pairs for metadata.\nFormat: key=value,key2=value2\n\n  Example: env=prod,tier=enterprise"
	}
	return "", ""
}

func (m *featuresModel) viewCreateForm(width int) string {
	return m.viewTwoPaneForm(width, "Create Feature", m.createFocused, func() ([]formRow, string) {
		rows := []formRow{
			{"Key:", featFieldKey, true},
			{"Name:", featFieldName, true},
			{"Meter Slug:", featFieldMeterSlug, false},
			{"Group By:", featFieldGroupByFilters, false},
			{"Metadata:", featFieldMetadata, false},
		}
		title, body := m.createFieldHelp()
		return rows, helpTitleRender(title) + "\n\n" + helpBodyRender(body)
	}, func(r formRow, inputW int) string {
		m.createFields[r.logical].Width = inputW
		return m.createFields[r.logical].View()
	}, m.createStatus)
}

// ─── two-pane form (same pattern as meters/customers) ───────────────────────

func (m *featuresModel) viewTwoPaneForm(
	width int,
	title string,
	focused int,
	buildRows func() ([]formRow, string),
	renderField func(formRow, int) string,
	status string,
) string {
	rightW := 40
	borderW := 1
	leftW := width - rightW - borderW - 2
	if leftW < 35 {
		leftW = 35
		rightW = width - leftW - borderW - 2
	}

	requiredLabel := labelStyle.Bold(true).Width(16)
	optionalLabel := labelStyle.Width(16)

	inputW := leftW - 20
	if inputW < 10 {
		inputW = 10
	}

	rows, helpContent := buildRows()

	var leftLines []string
	leftLines = append(leftLines, titleStyle.Render(title))
	leftLines = append(leftLines, "")

	for _, r := range rows {
		marker := "  "
		if focused == r.logical {
			marker = focusStyle.Render("▸ ")
		}
		lbl := optionalLabel
		if r.required {
			lbl = requiredLabel
		}
		leftLines = append(leftLines, marker+lbl.Render(r.label)+" "+renderField(r, inputW))
	}

	leftLines = append(leftLines, "")
	if status != "" {
		leftLines = append(leftLines, status)
	}
	leftLines = append(leftLines, descStyle.Render("↑↓/Tab fields • Enter submit • Esc cancel"))

	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colBorder).
		PaddingRight(1).
		Render(strings.Join(leftLines, "\n"))

	rightPane := lipgloss.NewStyle().
		Width(rightW).
		PaddingLeft(1).
		Render(helpContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

// ─── commands ─────────────────────────────────────────────────────────────────

func (m *featuresModel) load() tea.Cmd {
	return func() tea.Msg {
		raw, err := m.client.ListFeatures(m.showArchived)
		if err != nil {
			return featuresLoadedMsg{err: err, at: time.Now()}
		}

		// ListFeaturesResult is oneOf: plain array or paginated {items:[...]}
		features, err := parseFeaturesResponse(raw)
		if err != nil {
			return featuresLoadedMsg{err: err, at: time.Now()}
		}

		rows := make([][]string, len(features))
		ids := make([]string, len(features))
		for i, f := range features {
			gbStr := ""
			for k, v := range f.MeterGroupByFilters {
				if gbStr != "" {
					gbStr += ", "
				}
				gbStr += k + "=" + v
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
	ID                  string            `json:"id"`
	Key                 string            `json:"key"`
	Name                string            `json:"name"`
	MeterSlug           string            `json:"meterSlug"`
	MeterGroupByFilters map[string]string `json:"meterGroupByFilters"`
	CreatedAt           string            `json:"createdAt"`
	ArchivedAt          string            `json:"archivedAt"`
}

func parseFeaturesResponse(raw json.RawMessage) ([]featureListItem, error) {
	// Try plain array first
	var items []featureListItem
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	// Try paginated response
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
			ID                  string            `json:"id"`
			Key                 string            `json:"key"`
			Name                string            `json:"name"`
			MeterSlug           string            `json:"meterSlug"`
			MeterGroupByFilters map[string]string `json:"meterGroupByFilters"`
			Metadata            map[string]string `json:"metadata"`
			CreatedAt           string            `json:"createdAt"`
			UpdatedAt           string            `json:"updatedAt"`
			ArchivedAt          string            `json:"archivedAt"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return featureDetailMsg{err: fmt.Errorf("parse feature: %w", err)}
		}
		return featureDetailMsg{info: featureInfo{
			ID:                 f.ID,
			Key:                f.Key,
			Name:               f.Name,
			MeterSlug:          f.MeterSlug,
			MeterGroupByFilter: f.MeterGroupByFilters,
			Metadata:           f.Metadata,
			CreatedAt:          f.CreatedAt,
			UpdatedAt:          f.UpdatedAt,
			ArchivedAt:         f.ArchivedAt,
		}}
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func parseKVPairs(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}
