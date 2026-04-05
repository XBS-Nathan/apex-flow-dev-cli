package db

import "testing"

func FuzzSanitizeDBName(f *testing.F) {
	f.Add("mydb")
	f.Add("My-App.v2")
	f.Add("")
	f.Add("DROP TABLE users;")
	f.Add("db`name")
	f.Add("名前")

	f.Fuzz(func(t *testing.T, input string) {
		result := sanitizeDBName(input)

		// Result must only contain [a-z0-9_]
		for i := 0; i < len(result); i++ {
			c := result[i]
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				t.Errorf("sanitizeDBName(%q) = %q, contains invalid char %q at index %d",
					input, result, string(c), i)
			}
		}

		// Result must not be longer than input
		if len(result) > len(input) {
			t.Errorf("sanitizeDBName(%q) = %q, result longer than input", input, result)
		}

		// Idempotent: sanitizing the result should return the same thing
		if sanitizeDBName(result) != result {
			t.Errorf("sanitizeDBName is not idempotent: sanitizeDBName(%q) = %q, but sanitizeDBName(%q) = %q",
				input, result, result, sanitizeDBName(result))
		}
	})
}
