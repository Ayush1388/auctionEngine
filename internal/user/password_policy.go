package user

import (
	"strings"
	"unicode/utf8"

	"github.com/Ayush1388/auctionEngine/internal/validation"
)

// The password policy, shown to people on the sign-up form (frontend/app/passwordPolicy.js
// mirrors it; keep the two in step).
//
//   - 15 to 128 characters: length is what makes a password hard to guess, so there is
//     no "one capital, one symbol" rule (NIST SP 800-63B).
//   - not made of one repeated character ("aaaaaaaaaaaaaaa").
//   - does not contain the part of your email before the @ (when it is 4 or more characters).
const (
	PasswordMinLength = 15
	PasswordMaxLength = 128
)

// ValidatePassword returns a *validation.Error for the "password" field, or nil.
func ValidatePassword(email, password string) error {
	var problems validation.Error
	n := utf8.RuneCountInString(password)

	switch {
	case n < PasswordMinLength:
		problems.Add("password", "must be at least 15 characters")
	case n > PasswordMaxLength:
		problems.Add("password", "must be at most 128 characters")
	case strings.Count(password, string([]rune(password)[0])) == len([]rune(password)):
		problems.Add("password", "must not be one repeated character")
	default:
		local := strings.ToLower(strings.SplitN(strings.TrimSpace(email), "@", 2)[0])
		if len(local) >= 4 && strings.Contains(strings.ToLower(password), local) {
			problems.Add("password", "must not contain your email name")
		}
	}

	return problems.OrNil()
}
