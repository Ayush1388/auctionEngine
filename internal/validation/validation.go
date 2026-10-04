// Package validation turns struct-tag validation failures into readable,
// per-field messages that handlers can return to clients.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ErrInvalid matches any *Error with errors.Is.
var ErrInvalid = errors.New("invalid input")

// Error maps field names (as they appear in JSON) to what is wrong with them.
type Error struct {
	Fields map[string]string
}

func (e *Error) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " " + e.Fields[k]
	}

	return "invalid input: " + strings.Join(parts, "; ")
}

func (e *Error) Is(target error) bool {
	return target == ErrInvalid
}

// Add records a problem with a field. The first message for a field wins.
func (e *Error) Add(field, message string) {
	if e.Fields == nil {
		e.Fields = map[string]string{}
	}
	if _, exists := e.Fields[field]; !exists {
		e.Fields[field] = message
	}
}

// OrNil returns e as an error, or nil when no problems were recorded, so
// callers can write `return problems.OrNil()`.
func (e *Error) OrNil() error {
	if e == nil || len(e.Fields) == 0 {
		return nil
	}
	return e
}

type Validator struct {
	validate *validator.Validate
}

func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Report fields by their JSON name ("starting_price", not "StartingPrice").
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})

	return &Validator{validate: v}
}

// Struct validates s and returns nil, a *Error, or an unexpected error
// (for example when s is not a struct).
func (v *Validator) Struct(s any) error {
	err := v.validate.Struct(s)
	if err == nil {
		return nil
	}

	var fieldErrs validator.ValidationErrors
	if !errors.As(err, &fieldErrs) {
		return err
	}

	problems := &Error{}
	for _, fe := range fieldErrs {
		problems.Add(fieldPath(fe), message(fe))
	}

	return problems
}

// fieldPath returns the dotted JSON path of the field, without the name of
// the top-level struct: "item.name" rather than "CreateInput.item.name".
func fieldPath(fe validator.FieldError) string {
	namespace := fe.Namespace()
	if i := strings.Index(namespace, "."); i >= 0 {
		return namespace[i+1:]
	}
	return fe.Field()
}

func message(fe validator.FieldError) string {
	isString := fe.Kind() == reflect.String

	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		if isString {
			return fmt.Sprintf("must be at least %s characters", fe.Param())
		}
		return "must be at least " + fe.Param()
	case "max":
		if isString {
			return fmt.Sprintf("must be at most %s characters", fe.Param())
		}
		return "must be at most " + fe.Param()
	case "gte":
		return "must be at least " + fe.Param()
	case "gt":
		return "must be greater than " + fe.Param()
	case "lte":
		return "must be at most " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	default:
		return "is invalid"
	}
}
