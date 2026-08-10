package orchestrator

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/f1nniboy/lrcmux/internal/providers"
)

var ErrInvalidSource = errors.New("invalid source")

func filterBySources(provs []providers.Provider, sources []string) ([]providers.Provider, error) {
	if len(sources) == 0 {
		return provs, nil
	}

	var include, exclude []string
	for _, s := range sources {
		s = strings.ToLower(strings.TrimSpace(s))
		name, isExclude := strings.CutPrefix(s, "!")
		if name == "" {
			return nil, fmt.Errorf("%w: empty source name", ErrInvalidSource)
		}
		if isExclude {
			exclude = append(exclude, name)
		} else {
			include = append(include, name)
		}
	}

	if len(include) > 0 && len(exclude) > 0 {
		return nil, fmt.Errorf("%w: cannot mix include and exclude", ErrInvalidSource)
	}

	var out []providers.Provider
	if len(include) > 0 {
		out = slices.DeleteFunc(slices.Clone(provs), func(p providers.Provider) bool {
			return !slices.Contains(include, p.ID())
		})
	} else {
		out = slices.DeleteFunc(slices.Clone(provs), func(p providers.Provider) bool {
			return slices.Contains(exclude, p.ID())
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no providers match the requested filter", ErrInvalidSource)
	}
	return out, nil
}
