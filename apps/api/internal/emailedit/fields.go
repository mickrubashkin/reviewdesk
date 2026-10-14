package emailedit

import "encoding/json"

const (
	FieldTypeText   = "text"
	FieldTypeURL    = "url"
	FieldTypeImage  = "image"
	FieldTypeNumber = "number"
)

type EditableField struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
	Order int    `json:"order,omitempty"`
}

type EditableFields map[string]EditableField

func (fields EditableFields) JSON() ([]byte, error) {
	if fields == nil {
		return []byte("{}"), nil
	}

	return json.Marshal(fields)
}
