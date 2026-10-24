package user

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name, email, password string
		ok                    bool
	}{
		{"long enough", "ayush@example.com", "correct horse battery staple", true},
		{"exactly 15", "a@b.co", "abcdefghijklmno", true},
		{"too short", "a@b.co", "short-pass-14c", false},
		{"too long", "a@b.co", string(make([]rune, 129)), false},
		{"one repeated character", "a@b.co", "aaaaaaaaaaaaaaaa", false},
		{"contains email name", "ayushji@example.com", "my-AYUSHJI-secret-pass", false},
		{"short email name is fine", "al@example.com", "al-and-a-long-passphrase", true},
	}
	for _, c := range cases {
		err := ValidatePassword(c.email, c.password)
		if (err == nil) != c.ok {
			t.Errorf("%s: ok=%v, got err=%v", c.name, c.ok, err)
		}
	}
}
