package sortopts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grafana/grafana/pkg/services/search/model"
	"github.com/grafana/grafana/pkg/util/errutil"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	SortOptionsByQueryParam = map[string]model.SortOption{
		"login-asc":        newSortOption("login", false, 0),
		"login-desc":       newSortOption("login", true, 0),
		"email-asc":        newSortOption("email", false, 1),
		"email-desc":       newSortOption("email", true, 1),
		"name-asc":         newSortOption("name", false, 2),
		"name-desc":        newSortOption("name", true, 2),
		"last_active-asc":  newSortOption("last_active", false, 3),
		"last_active-desc": newSortOption("last_active", true, 3),
	}

	ErrorUnknownSortingOption = errutil.BadRequest("unknown sorting option")
)

type Sorter struct {
	Field      string
	Descending bool
}

func (s Sorter) OrderBy() string {
	if s.Descending {
		return fmt.Sprintf("u.%v DESC", s.Field)
	}

	return fmt.Sprintf("u.%v ASC", s.Field)
}

func newSortOption(field string, desc bool, index int) model.SortOption {
	direction := "asc"
	alpha := ("A-Z")
	if desc {
		direction = "desc"
		alpha = ("Z-A")
	}
	return model.SortOption{
		Name:        fmt.Sprintf("%v-%v", field, direction),
		DisplayName: fmt.Sprintf("%v (%v)", cases.Title(language.Und).String(field), alpha),
		Description: fmt.Sprintf("Sort %v in an alphabetically %vending order", field, direction),
		Index:       index,
		Filter:      []model.SortOptionFilter{Sorter{Field: field, Descending: desc}},
	}
}

// ParseSortQueryParam parses the "sort" query param and returns an ordered list of SortOption(s)
func ParseSortQueryParam(param string) ([]model.SortOption, error) {
	opts := []model.SortOption{}
	if param != "" {
		optsStr := strings.Split(param, ",")
		for i := range optsStr {
			if opt, ok := SortOptionsByQueryParam[optsStr[i]]; !ok {
				return nil, ErrorUnknownSortingOption.Errorf("%v option unknown", optsStr[i])
			} else {
				opts = append(opts, opt)
			}
		}
		sort.Slice(opts, func(i, j int) bool {
			return opts[i].Index < opts[j].Index || (opts[i].Index == opts[j].Index && opts[i].Name < opts[j].Name)
		})
	}
	return opts, nil
}
