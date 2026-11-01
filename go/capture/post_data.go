package capture

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"github.com/romayengineer/fme/go/core/orderedmap"
)

// GetPostDataMap parses a form-encoded GraphQL post body, preserving key order.
func GetPostDataMap(request playwright.Request) (orderedmap.OrderedMap, error) {
	om := orderedmap.NewOrderedMap()
	data, err := request.PostData()
	if err != nil {
		return om, fmt.Errorf("read request post data: %w", err)
	}
	decoded, err := url.QueryUnescape(data)
	if err != nil {
		return om, fmt.Errorf("unescape request post data: %w", err)
	}
	for p := range strings.SplitSeq(decoded, "&") {
		keyValue := strings.SplitN(p, "=", 2)
		if len(keyValue) < 2 {
			return om, fmt.Errorf("malformed post-data field %q", p)
		}
		om.Set(keyValue[0], keyValue[1])
	}
	return om, nil
}

// GetPostData rebuilds an application/x-www-form-urlencoded body from the parsed
// post-data map. GetPostDataMap already unescaped the values, so we must re-encode
// them here - previously the value was unescaped a second time, which corrupted the
// payload (and it was never re-encoded as a form body).
func GetPostData(postData orderedmap.OrderedMap) (string, error) {
	params := url.Values{}
	for _, key := range postData.Keys() {
		value, exists := postData.Get(key)
		if !exists {
			continue
		}
		params.Set(key, value)
	}
	return params.Encode(), nil
}

// GetPostDataTargetID extracts variables.targetId from a parsed GraphQL
// post-data map.
func GetPostDataTargetID(postData orderedmap.OrderedMap) string {
	varsStr, ok := postData.Get("variables")
	if !ok {
		return ""
	}

	var vars map[string]any
	if err := json.Unmarshal([]byte(varsStr), &vars); err != nil {
		return ""
	}

	targetID, _ := vars["targetId"].(string)
	return targetID
}
