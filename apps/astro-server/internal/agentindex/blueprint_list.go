package agentindex

import (
	"fmt"
	"strings"
)

// BlueprintListOptions filters and sorts for blueprint list queries.
type BlueprintListOptions struct {
	Query      string
	Tag        string
	Visibility string
	Sort       string
	Limit      int // 0 = no limit (internal callers only)
	Offset     int
}

// BlueprintListPage is a paginated blueprint list result.
type BlueprintListPage struct {
	Agents []*Agent
	Total  int
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func likePattern(query string) string {
	if query == "" {
		return ""
	}
	return "%" + escapeLike(query) + "%"
}

const latestVersionSubquery = `
	lv.published_at = (
		SELECT MAX(v2.published_at) FROM agent_versions v2
		WHERE v2.account_id = lv.account_id AND v2.name = lv.name
	)`

// blueprintColumnPrefix qualifies agents-table columns for correlated subqueries.
func blueprintColumnPrefix(tableAlias string) string {
	if tableAlias != "" {
		return tableAlias + "."
	}
	return "agents."
}

// blueprintOrderClause orders list rows. When versionAlias is set (list queries join the latest
// version as v), sort=newest uses v.published_at instead of a per-row correlated subquery.
func blueprintOrderClause(sort, agentAlias, versionAlias string) string {
	switch sort {
	case "newest":
		if versionAlias != "" {
			return fmt.Sprintf("ORDER BY %s.published_at DESC NULLS LAST, %s.name", versionAlias, agentAlias)
		}
		prefix := blueprintColumnPrefix(agentAlias)
		return fmt.Sprintf(`ORDER BY (
			SELECT MAX(v.published_at) FROM agent_versions v
			WHERE v.account_id = %saccount_id AND v.name = %sname
		) DESC NULLS LAST, %sname`, prefix, prefix, prefix)
	default:
		return fmt.Sprintf("ORDER BY %s.name", agentAlias)
	}
}

func appendBlueprintTextFilter(where *[]string, args *[]any, argN *int, tableAlias, query string) {
	if query == "" {
		return
	}
	pattern := likePattern(query)
	col := blueprintColumnPrefix(tableAlias)
	*where = append(*where, fmt.Sprintf(`(%sname ILIKE $%d ESCAPE '\' OR EXISTS (
		SELECT 1 FROM agent_versions lv
		WHERE lv.account_id = %saccount_id AND lv.name = %sname
		AND `+latestVersionSubquery+`
		AND (
			lv.agent_card_json->>'description' ILIKE $%d ESCAPE '\'
			OR EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(COALESCE(lv.agent_card_json->'tags', '[]'::jsonb)) AS tag_elem
				WHERE tag_elem ILIKE $%d ESCAPE '\'
			)
		)
	))`, col, *argN, col, col, *argN, *argN))
	*args = append(*args, pattern)
	*argN++
}

func appendBlueprintTagFilter(where *[]string, args *[]any, argN *int, tableAlias, tag string) {
	if tag == "" {
		return
	}
	col := blueprintColumnPrefix(tableAlias)
	*where = append(*where, fmt.Sprintf(`EXISTS (
		SELECT 1 FROM agent_versions lv
		WHERE lv.account_id = %saccount_id AND lv.name = %sname
		AND `+latestVersionSubquery+`
		AND EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(COALESCE(lv.agent_card_json->'tags', '[]'::jsonb)) AS tag_elem
			WHERE lower(tag_elem) = lower($%d)
		)
	)`, col, col, *argN))
	*args = append(*args, tag)
	*argN++
}
