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
	// Shared means the value comes from the shared text with the same key in
	// the email's language; Value keeps a copy so rendering stays the same.
	Shared bool `json:"shared,omitempty"`
	// Format is FormatMarkdown for a text field written in Markdown; empty
	// for plain text. A field becomes Markdown when it is first edited with
	// formatting, so templates stay as they are.
	Format string `json:"format,omitempty"`
}

type EditableFields map[string]EditableField

func (fields EditableFields) JSON() ([]byte, error) {
	if fields == nil {
		return []byte("{}"), nil
	}

	return json.Marshal(fields)
}
