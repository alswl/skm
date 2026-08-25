package plugins

import "encoding/json"

// UnmarshalError decodes a plugin's `error` field, which is either the
// {code,message} object or a legacy bare string. The bare-string form gets
// defaultCode, so each protocol keeps its own fallback (a provider's failed
// fetch is not a target's failed install).
func UnmarshalError(data []byte, code, message *string, defaultCode string) error {
	var obj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && (obj.Code != "" || obj.Message != "") {
		*code, *message = obj.Code, obj.Message
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*code, *message = defaultCode, s
	return nil
}
