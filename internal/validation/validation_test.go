package validation

import (
	"errors"
	"testing"
)

type nested struct {
	Name string `json:"name" validate:"required"`
}

type input struct {
	Item     nested `json:"item"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=15"`
	Price    int64  `json:"starting_price" validate:"gte=0"`
}

func TestStruct(t *testing.T) {
	v := New()

	err := v.Struct(input{Email: "nope", Password: "short", Price: -1})

	var problems *Error
	if !errors.As(err, &problems) {
		t.Fatalf("got %T %v, want *Error", err, err)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("expected errors.Is(err, ErrInvalid)")
	}

	want := map[string]string{
		"email":          "must be a valid email address",
		"password":       "must be at least 15 characters",
		"starting_price": "must be at least 0",
		"item.name":      "is required",
	}
	for field, msg := range want {
		if got := problems.Fields[field]; got != msg {
			t.Errorf("%s: got %q, want %q", field, got, msg)
		}
	}

	if err := v.Struct(input{Item: nested{Name: "lamp"}, Email: "a@b.co", Password: "long-enough-password"}); err != nil {
		t.Fatalf("valid input: %v", err)
	}
}

func TestOrNil(t *testing.T) {
	var problems Error
	if problems.OrNil() != nil {
		t.Fatal("empty Error should be nil")
	}

	problems.Add("name", "is required")
	problems.Add("name", "ignored, first message wins")
	if err := problems.OrNil(); err == nil || problems.Fields["name"] != "is required" {
		t.Fatalf("got %v", err)
	}
}
