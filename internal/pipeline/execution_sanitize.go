package pipeline

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
)

const (
	executionReservedSecretKey = "secret"
	executionRedactedValue     = "[REDACTED]"
)

func SanitizeExecutionValue(value any, secretValues ...string) any {
	return sanitizeExecutionValueWithSecrets(value, normalizeExecutionSecretValues(secretValues))
}

func SanitizeExecutionJSON(raw json.RawMessage, secretValues ...string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	secrets := normalizeExecutionSecretValues(secretValues)

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		sanitized := sanitizeExecutionValueWithSecrets(decoded, secrets)
		if data, marshalErr := json.Marshal(sanitized); marshalErr == nil {
			return data
		}
	}

	redacted := redactExecutionSecretsInString(string(raw), secrets)
	return json.RawMessage(redacted)
}

func SanitizeExecutionJSONString(raw string, secretValues ...string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	sanitized := SanitizeExecutionJSON(json.RawMessage(raw), secretValues...)
	if len(sanitized) == 0 {
		return ""
	}

	return string(sanitized)
}

func sanitizeExecutionInputMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	sanitized, ok := sanitizeExecutionValueWithSecrets(input, executionSecretValuesFromInput(input)).(map[string]any)
	if !ok || sanitized == nil {
		return make(map[string]any)
	}

	return sanitized
}

func sanitizeExecutionResult(result *node.NodeResult, secretValues []string) *node.NodeResult {
	if result == nil {
		return nil
	}

	sanitized := &node.NodeResult{
		Error: result.Error,
	}
	if len(result.Output) > 0 {
		sanitized.Output = SanitizeExecutionJSON(result.Output, secretValues...)
	}
	if result.ReturnValue != nil {
		sanitized.ReturnValue = sanitizeExecutionValueWithSecrets(result.ReturnValue, secretValues)
	}

	return sanitized
}

func sanitizeExecutionValueWithSecrets(value any, secretValues []string) any {
	if value == nil {
		return nil
	}

	return sanitizeExecutionReflect(reflect.ValueOf(value), secretValues)
}

func sanitizeExecutionReflect(value reflect.Value, secretValues []string) any {
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return sanitizeExecutionReflect(value.Elem(), secretValues)
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if value.Type().Key().Kind() != reflect.String {
			return value.Interface()
		}

		sanitized := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if key == executionReservedSecretKey {
				continue
			}
			sanitized[key] = sanitizeExecutionReflect(iter.Value(), secretValues)
		}
		return sanitized
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return value.Interface()
		}
		sanitized := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			sanitized[i] = sanitizeExecutionReflect(value.Index(i), secretValues)
		}
		return sanitized
	case reflect.Array:
		sanitized := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			sanitized[i] = sanitizeExecutionReflect(value.Index(i), secretValues)
		}
		return sanitized
	case reflect.String:
		return redactExecutionSecretsInString(value.String(), secretValues)
	default:
		return value.Interface()
	}
}

func executionSecretValuesFromInput(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}

	rawSecrets, ok := input[executionReservedSecretKey]
	if !ok {
		return nil
	}

	return normalizeExecutionSecretValues(collectExecutionSecretStrings(rawSecrets))
}

func collectExecutionSecretStrings(value any) []string {
	if value == nil {
		return nil
	}

	return collectExecutionSecretStringsReflect(reflect.ValueOf(value))
}

func collectExecutionSecretStringsReflect(value reflect.Value) []string {
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return collectExecutionSecretStringsReflect(value.Elem())
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		values := make([]string, 0, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			values = append(values, collectExecutionSecretStringsReflect(iter.Value())...)
		}
		return values
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		values := make([]string, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			values = append(values, collectExecutionSecretStringsReflect(value.Index(i))...)
		}
		return values
	case reflect.String:
		if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	default:
		return nil
	}
}

func normalizeExecutionSecretValues(secretValues []string) []string {
	if len(secretValues) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(secretValues))
	normalized := make([]string, 0, len(secretValues))
	for _, secretValue := range secretValues {
		if strings.TrimSpace(secretValue) == "" {
			continue
		}
		if _, ok := seen[secretValue]; ok {
			continue
		}
		seen[secretValue] = struct{}{}
		normalized = append(normalized, secretValue)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return len(normalized[i]) > len(normalized[j])
	})

	return normalized
}

func redactExecutionSecretsInString(value string, secretValues []string) string {
	if value == "" || len(secretValues) == 0 {
		return value
	}

	redacted := value
	for _, secretValue := range secretValues {
		if secretValue == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secretValue, executionRedactedValue)
	}

	return redacted
}
