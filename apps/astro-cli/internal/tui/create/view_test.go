package create

import (
	"strings"
	"testing"
)

// testOptions returns a small option list for testing renderOptionList.
func testOptions() []option {
	return []option{
		{"First", "first", false},
		{"Second", "second", false},
		{"Third", "third", false},
	}
}

func TestRenderOptionList_radio_onlyOnCursorRow(t *testing.T) {
	opts := testOptions()
	for cursor := 0; cursor < len(opts); cursor++ {
		m := model{cursor: cursor, selected: make(map[int]bool)}
		out := m.renderOptionList(opts, true)
		count := strings.Count(out, "●")
		if count != 1 {
			t.Errorf("radio cursor=%d: expected exactly 1 ●, got %d", cursor, count)
		}
		lines := strings.Split(out, "\n")
		for i := 0; i < len(opts) && i < len(lines); i++ {
			hasFilled := strings.Contains(lines[i], "●")
			if i == cursor && !hasFilled {
				t.Errorf("radio cursor=%d: row %d should contain ●", cursor, i)
			}
			if i != cursor && hasFilled {
				t.Errorf("radio cursor=%d: row %d should not contain ●", cursor, i)
			}
		}
	}
}

func TestRenderOptionList_checkbox_filledWhereSelected(t *testing.T) {
	opts := testOptions()
	m := model{cursor: 1, selected: map[int]bool{0: true, 2: true}}
	out := m.renderOptionList(opts, false)
	lines := strings.Split(out, "\n")
	// Row 0 and 2 selected → ●; row 1 not selected → ○
	if !strings.Contains(lines[0], "●") {
		t.Error("checkbox: selected row 0 should contain ●")
	}
	if strings.Contains(lines[1], "●") {
		t.Error("checkbox: unselected row 1 should not contain ●")
	}
	if !strings.Contains(lines[2], "●") {
		t.Error("checkbox: selected row 2 should contain ●")
	}
}

func TestRenderOptionList_checkbox_multipleFilled(t *testing.T) {
	opts := testOptions()
	m := model{cursor: 0, selected: map[int]bool{0: true, 1: true}}
	out := m.renderOptionList(opts, false)
	count := strings.Count(out, "●")
	if count != 2 {
		t.Errorf("checkbox: expected 2 ● (two selected), got %d", count)
	}
}
