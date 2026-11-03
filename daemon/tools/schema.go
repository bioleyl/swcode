package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// SchemaType is a JSON schema's type.
type SchemaType string

const (
	TypeObject  SchemaType = "object"
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeBoolean SchemaType = "boolean"
	TypeArray   SchemaType = "array"
)

// Schema is the JSON schema of a tool's input, as the model reads it, and what the model's input is checked
// against: one description for both, as zod's was.
type Schema struct {
	Type        SchemaType
	Description string
	Enum        []string
	MinLength   int
	MaxLength   int
	Minimum     *float64
	Maximum     *float64
	Pattern     *regexp.Regexp
	// Format is a string's format: uri, for a web address.
	Format string
	// Trim takes the spaces off a string's ends before it is checked and used.
	Trim       bool
	Items      *Schema
	MinItems   int
	MaxItems   int
	Properties []Property
}

// Property is a field of an object, in the order the model reads them.
type Property struct {
	Name     string
	Schema   Schema
	Required bool
}

func Object(properties ...Property) Schema { return Schema{Properties: properties, Type: TypeObject} }
func String() Schema                       { return Schema{Type: TypeString} }
func Number() Schema                       { return Schema{Type: TypeNumber} }
func Boolean() Schema                      { return Schema{Type: TypeBoolean} }
func Array(items Schema) Schema            { return Schema{Items: &items, Type: TypeArray} }
func Enum(values ...string) Schema         { return Schema{Enum: values, Type: TypeString} }

func Required(name string, schema Schema) Property {
	return Property{Name: name, Required: true, Schema: schema}
}
func Optional(name string, schema Schema) Property { return Property{Name: name, Schema: schema} }

func (s Schema) Describe(description string) Schema { s.Description = description; return s }
func (s Schema) Min(length int) Schema {
	if s.Type == TypeArray {
		s.MinItems = length
	} else {
		s.MinLength = length
	}
	return s
}
func (s Schema) Max(length int) Schema {
	if s.Type == TypeArray {
		s.MaxItems = length
	} else {
		s.MaxLength = length
	}
	return s
}
func (s Schema) Between(minimum, maximum float64) Schema {
	s.Minimum, s.Maximum = &minimum, &maximum
	return s
}
func (s Schema) AtLeast(minimum float64) Schema { s.Minimum = &minimum; return s }
func (s Schema) Matching(pattern string) Schema { s.Pattern = regexp.MustCompile(pattern); return s }
func (s Schema) Trimmed() Schema                { s.Trim = true; return s }
func (s Schema) WebAddress() Schema             { s.Format = formatURI; return s }

const formatURI = "uri"

var webAddress = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)

// field writes one key and its value, keeping the order the model reads them in.
func field(buffer *bytes.Buffer, key string, value any) {
	if buffer.Len() > 1 {
		buffer.WriteByte(',')
	}
	name, _ := json.Marshal(key)
	text, _ := json.Marshal(value)
	buffer.Write(name)
	buffer.WriteByte(':')
	buffer.Write(text)
}

// MarshalJSON writes the schema as zod's toJSONSchema does, its keys in a steady order.
func (s Schema) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString("{")
	field(buffer, "type", s.Type)
	if len(s.Enum) > 0 {
		field(buffer, "enum", s.Enum)
	}
	if s.MinLength > 0 {
		field(buffer, "minLength", s.MinLength)
	}
	if s.MaxLength > 0 {
		field(buffer, "maxLength", s.MaxLength)
	}
	if s.Minimum != nil {
		field(buffer, "minimum", *s.Minimum)
	}
	if s.Maximum != nil {
		field(buffer, "maximum", *s.Maximum)
	}
	if s.Format != "" {
		field(buffer, "format", s.Format)
	}
	if s.Pattern != nil {
		field(buffer, "pattern", s.Pattern.String())
	}
	if s.Items != nil {
		field(buffer, "items", s.Items)
	}
	if s.MinItems > 0 {
		field(buffer, "minItems", s.MinItems)
	}
	if s.MaxItems > 0 {
		field(buffer, "maxItems", s.MaxItems)
	}
	if s.Type == TypeObject {
		s.objectFields(buffer)
	}
	if s.Description != "" {
		field(buffer, "description", s.Description)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func (s Schema) objectFields(buffer *bytes.Buffer) {
	properties := bytes.NewBufferString("{")
	required := []string{}
	for _, property := range s.Properties {
		field(properties, property.Name, property.Schema)
		if property.Required {
			required = append(required, property.Name)
		}
	}
	properties.WriteByte('}')
	if buffer.Len() > 1 {
		buffer.WriteByte(',')
	}
	buffer.WriteString(`"properties":`)
	buffer.Write(properties.Bytes())
	if len(required) > 0 {
		field(buffer, "required", required)
	}
	field(buffer, "additionalProperties", false)
}

// Parameters is the schema as the model's tool definition takes it.
func (s Schema) Parameters() map[string]any {
	data, _ := json.Marshal(s)
	var parameters map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	_ = decoder.Decode(&parameters)
	return parameters
}

// Check checks the model's input against the schema, trimming what is trimmed: the input as checked, or what does
// not fit, a line a problem.
func (s Schema) Check(input any) (any, error) {
	problems := []string{}
	checked := s.check(input, "", &problems)
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return checked, nil
}

func at(path string) string {
	if path == "" {
		return "the input"
	}
	return path
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func (s Schema) check(input any, path string, problems *[]string) any {
	fail := func(problem string) any {
		*problems = append(*problems, fmt.Sprintf("✖ %s\n  → at %s", problem, at(path)))
		return input
	}
	switch s.Type {
	case TypeObject:
		object, ok := input.(map[string]any)
		if !ok {
			return fail("Invalid input: expected object")
		}
		return s.checkObject(object, path, problems)
	case TypeString:
		text, ok := input.(string)
		if !ok {
			return fail("Invalid input: expected string")
		}
		return s.checkString(text, fail)
	case TypeNumber:
		number, ok := input.(float64)
		if !ok {
			return fail("Invalid input: expected number")
		}
		return s.checkNumber(number, fail)
	case TypeBoolean:
		if _, ok := input.(bool); !ok {
			return fail("Invalid input: expected boolean")
		}
		return input
	case TypeArray:
		items, ok := input.([]any)
		if !ok {
			return fail("Invalid input: expected array")
		}
		return s.checkArray(items, path, problems, fail)
	}
	return input
}

func (s Schema) checkObject(object map[string]any, path string, problems *[]string) any {
	checked := map[string]any{}
	known := map[string]bool{}
	for _, property := range s.Properties {
		known[property.Name] = true
		value, present := object[property.Name]
		if !present || value == nil {
			if property.Required {
				*problems = append(*problems, fmt.Sprintf("✖ Invalid input: expected %s, received undefined\n  → at %s", property.Schema.Type, join(path, property.Name)))
			}
			continue
		}
		checked[property.Name] = property.Schema.check(value, join(path, property.Name), problems)
	}
	unknown := []string{}
	for key := range object {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)
	for _, key := range unknown {
		*problems = append(*problems, fmt.Sprintf("✖ Unrecognized key: %q\n  → at %s", key, at(path)))
	}
	return checked
}

func (s Schema) checkString(text string, fail func(string) any) any {
	if s.Trim {
		text = strings.TrimSpace(text)
	}
	length := utf8.RuneCountInString(text)
	switch {
	case s.MinLength > 0 && length < s.MinLength:
		return fail(fmt.Sprintf("Too small: expected string to have >=%d characters", s.MinLength))
	case s.MaxLength > 0 && length > s.MaxLength:
		return fail(fmt.Sprintf("Too big: expected string to have <=%d characters", s.MaxLength))
	case len(s.Enum) > 0 && !slices.Contains(s.Enum, text):
		return fail(fmt.Sprintf("Invalid option: expected one of %s", strings.Join(quoted(s.Enum), "|")))
	case s.Pattern != nil && !s.Pattern.MatchString(text):
		return fail("Invalid string: must match pattern " + s.Pattern.String())
	case s.Format == formatURI && !webAddress.MatchString(text):
		return fail("Invalid URL: an http or https address")
	}
	return text
}

func quoted(values []string) []string {
	texts := make([]string, len(values))
	for index, value := range values {
		texts[index] = `"` + value + `"`
	}
	return texts
}

func (s Schema) checkNumber(number float64, fail func(string) any) any {
	if s.Minimum != nil && number < *s.Minimum {
		return fail(fmt.Sprintf("Too small: expected number to be >=%v", *s.Minimum))
	}
	if s.Maximum != nil && number > *s.Maximum {
		return fail(fmt.Sprintf("Too big: expected number to be <=%v", *s.Maximum))
	}
	return number
}

func (s Schema) checkArray(items []any, path string, problems *[]string, fail func(string) any) any {
	if s.MinItems > 0 && len(items) < s.MinItems {
		return fail(fmt.Sprintf("Too small: expected array to have >=%d items", s.MinItems))
	}
	if s.MaxItems > 0 && len(items) > s.MaxItems {
		return fail(fmt.Sprintf("Too big: expected array to have <=%d items", s.MaxItems))
	}
	checked := make([]any, len(items))
	for index, item := range items {
		checked[index] = s.Items.check(item, fmt.Sprintf("%s[%d]", at(path), index), problems)
	}
	return checked
}
