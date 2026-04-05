package config

import "testing"

func FuzzDbNameFromDir(f *testing.F) {
	f.Add("/home/user/projects/myapp")
	f.Add("/home/user/projects/My-App.v2")
	f.Add("/tmp/a")
	f.Add("/")
	f.Add("")
	f.Add("/home/user/projects/名前")

	f.Fuzz(func(t *testing.T, input string) {
		result := dbNameFromDir(input)

		// Result must only contain [a-z0-9_]
		for i := 0; i < len(result); i++ {
			c := result[i]
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				t.Errorf("dbNameFromDir(%q) = %q, contains invalid char %q at index %d",
					input, result, string(c), i)
			}
		}

		// Idempotent when applied to the result as a dir name (skip empty results)
		if result != "" && dbNameFromDir("/fake/"+result) != result {
			t.Errorf("dbNameFromDir is not idempotent on result: input=%q result=%q re-result=%q",
				input, result, dbNameFromDir("/fake/"+result))
		}
	})
}
