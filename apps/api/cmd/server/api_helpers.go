package main

func emptyMapIfNil(value any) any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
