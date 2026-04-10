package templating

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

type pathToken struct {
	key   string
	index *int
}

// RenderString resolves {{template}} expressions against the provided input.
// Nested map paths and array indexes such as {{input.nodes[0].status}} are supported.
func RenderString(value string, input map[string]any) (string, error) {
	matches := placeholderPattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}

	context := make(map[string]any, len(input)+1)
	for key, item := range input {
		context[key] = item
	}
	context["input"] = input

	var builder strings.Builder
	lastIndex := 0

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		builder.WriteString(value[lastIndex:match[0]])
		expression := strings.TrimSpace(value[match[2]:match[3]])

		resolved, err := resolveExpression(context, expression)
		if err != nil {
			return "", err
		}

		rendered, err := stringifyValue(resolved)
		if err != nil {
			return "", err
		}

		builder.WriteString(rendered)
		lastIndex = match[1]
	}

	builder.WriteString(value[lastIndex:])
	return builder.String(), nil
}

// RenderStrings walks structs, slices, and string maps to render templates in string values.
func RenderStrings(target any, input map[string]any) error {
	if target == nil {
		return nil
	}

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	return renderValue(value.Elem(), input)
}

// RenderJSON renders template placeholders across arbitrary JSON-compatible
// payloads and re-encodes the rendered value back into canonical JSON.
func RenderJSON(payload json.RawMessage, input map[string]any) (json.RawMessage, error) {
	if len(payload) == 0 {
		return json.RawMessage("{}"), nil
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, fmt.Errorf("decode json payload: %w", err)
	}

	if err := RenderStrings(&value, input); err != nil {
		return nil, err
	}

	rendered, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode json payload: %w", err)
	}

	return rendered, nil
}

func renderValue(value reflect.Value, input map[string]any) error {
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return renderValue(value.Elem(), input)
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}

		elem := value.Elem()
		copyValue := reflect.New(elem.Type()).Elem()
		copyValue.Set(elem)
		if err := renderValue(copyValue, input); err != nil {
			return err
		}
		value.Set(copyValue)
		return nil
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if !field.CanSet() {
				continue
			}
			if err := renderValue(field, input); err != nil {
				return err
			}
		}
		return nil
	case reflect.String:
		rendered, err := RenderString(value.String(), input)
		if err != nil {
			return err
		}
		value.SetString(rendered)
		return nil
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < value.Len(); i++ {
			if err := renderValue(value.Index(i), input); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := renderValue(value.Index(i), input); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if value.IsNil() || value.Type().Key().Kind() != reflect.String {
			return nil
		}

		for _, key := range value.MapKeys() {
			item := value.MapIndex(key)
			copyValue := reflect.New(value.Type().Elem()).Elem()
			copyValue.Set(item)
			if err := renderValue(copyValue, input); err != nil {
				return err
			}
			value.SetMapIndex(key, copyValue)
		}
		return nil
	default:
		return nil
	}
}

func resolveExpression(context map[string]any, expression string) (any, error) {
	tokens, err := parseExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("template %q: %w", expression, err)
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("template %q: empty expression", expression)
	}

	rootToken := tokens[0]
	current, ok := context[rootToken.key]
	if !ok {
		return nil, fmt.Errorf("template %q: %s not found", expression, rootToken.key)
	}

	if rootToken.index != nil {
		current, err = resolveIndex(current, *rootToken.index)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", expression, err)
		}
	}

	for _, token := range tokens[1:] {
		if token.index != nil {
			current, err = resolveIndex(current, *token.index)
			if err != nil {
				return nil, fmt.Errorf("template %q: %w", expression, err)
			}
			continue
		}

		current, err = resolveKey(current, token.key)
		if err != nil {
			return nil, fmt.Errorf("template %q: %w", expression, err)
		}
	}

	return current, nil
}

func parseExpression(expression string) ([]pathToken, error) {
	var tokens []pathToken

	for i := 0; i < len(expression); {
		switch expression[i] {
		case '.':
			i++
			continue
		case '[':
			end := strings.IndexByte(expression[i:], ']')
			if end == -1 {
				return nil, fmt.Errorf("missing closing bracket")
			}

			content := strings.TrimSpace(expression[i+1 : i+end])
			if content == "" {
				return nil, fmt.Errorf("empty array index")
			}

			if content[0] == '"' || content[0] == '\'' {
				if len(content) < 2 || content[len(content)-1] != content[0] {
					return nil, fmt.Errorf("invalid quoted key")
				}
				tokens = append(tokens, pathToken{key: content[1 : len(content)-1]})
			} else {
				index, err := strconv.Atoi(content)
				if err != nil {
					return nil, fmt.Errorf("invalid array index %q", content)
				}
				tokens = append(tokens, pathToken{index: &index})
			}

			i += end + 1
		default:
			start := i
			for i < len(expression) {
				ch := expression[i]
				if ch == '.' || ch == '[' {
					break
				}
				i++
			}

			segment := strings.TrimSpace(expression[start:i])
			if segment == "" {
				return nil, fmt.Errorf("invalid path")
			}

			tokens = append(tokens, pathToken{key: segment})
		}
	}

	return tokens, nil
}

func resolveKey(current any, key string) (any, error) {
	switch typed := current.(type) {
	case map[string]any:
		value, ok := typed[key]
		if !ok {
			return nil, fmt.Errorf("key %q not found", key)
		}
		return value, nil
	}

	value := reflect.ValueOf(current)
	if !value.IsValid() {
		return nil, fmt.Errorf("cannot access key %q on nil", key)
	}

	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("cannot access key %q on %T", key, current)
		}

		item := value.MapIndex(reflect.ValueOf(key))
		if !item.IsValid() {
			return nil, fmt.Errorf("key %q not found", key)
		}
		return item.Interface(), nil
	case reflect.Struct:
		field := value.FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, key)
		})
		if !field.IsValid() {
			return nil, fmt.Errorf("field %q not found", key)
		}
		return field.Interface(), nil
	default:
		return nil, fmt.Errorf("cannot access key %q on %T", key, current)
	}
}

func resolveIndex(current any, index int) (any, error) {
	switch typed := current.(type) {
	case []any:
		if index < 0 || index >= len(typed) {
			return nil, fmt.Errorf("index %d out of range", index)
		}
		return typed[index], nil
	}

	value := reflect.ValueOf(current)
	if !value.IsValid() {
		return nil, fmt.Errorf("cannot access index %d on nil", index)
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		if index < 0 || index >= value.Len() {
			return nil, fmt.Errorf("index %d out of range", index)
		}
		return value.Index(index).Interface(), nil
	default:
		return nil, fmt.Errorf("cannot access index %d on %T", index, current)
	}
}

func stringifyValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.Number:
		return typed.String(), nil
	case fmt.Stringer:
		return typed.String(), nil
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", typed), nil
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("marshal template value: %w", err)
		}
		return string(data), nil
	}
}
