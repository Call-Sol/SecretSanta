package telegram

import (
	"strconv"
	"strings"
)

func parseCallback(data string) (string, []int64, bool) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 || len(data) > 64 {
		return "", nil, false
	}
	verb := parts[0] + ":" + parts[1]
	switch verb {
	case "menu:home", "menu:help", "grp:new", "grp:list":
		return verb, nil, len(parts) == 2
	case "grp:open", "grp:members", "inv:accept", "game:open", "game:create", "game:confirm", "game:decline", "game:people":
		if len(parts) != 3 {
			return "", nil, false
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || id < 1 {
			return "", nil, false
		}
		return verb, []int64{id}, true
	default:
		return "", nil, false
	}
}
