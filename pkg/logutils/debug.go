package logutils

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kr/pretty"
)

func PrettyJSON(v any) string {
	var raw []byte

	switch val := v.(type) {
	case []byte:
		raw = val
	case string:
		raw = []byte(val)
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("[logutils.PrettyJSON marshal error: %v]", err)
		}
		raw = b
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func PrettyPrint(v any) {
	pretty.Println(PrettyJSON(v))
}
