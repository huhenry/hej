package log

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func loggingHandler(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) == "POST" {
		levels, ok := r.URL.Query()["level"]
		if !ok || len(levels) < 1 {
			w.Write([]byte("Url Param 'level' is missing"))
			return
		}
		level := levels[0]
		err := UpdateScopes(level)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}
	}
	scopes := Scopes()
	lines := make([]string, 0, len(scopes))
	for name, scope := range scopes {
		lines = append(lines, fmt.Sprintf("%s: %s", name, scope.GetOutputLevel()))
	}
	sort.Strings(lines)
	lines = append(lines, "")
	data := strings.Join(lines, "\n")
	w.Write([]byte(data))
}

func init() {
	http.HandleFunc("/debug/logging", loggingHandler)
}
