package postgresql

import (
	"reflect"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{
			name:   "simple statements",
			script: "CREATE TABLE a (id TEXT);\nCREATE TABLE b (id TEXT);\n",
			want: []string{
				"CREATE TABLE a (id TEXT);",
				"\nCREATE TABLE b (id TEXT);",
			},
		},
		{
			name:   "semicolon inside a string literal is not a split point",
			script: "INSERT INTO t (v) VALUES ('a;b');\nSELECT 1;",
			want: []string{
				"INSERT INTO t (v) VALUES ('a;b');",
				"\nSELECT 1;",
			},
		},
		{
			name:   "apostrophe in a line comment does not desync quote state",
			script: "-- financial-tracker's own schema\nCREATE TABLE a (id TEXT);\nCREATE TABLE b (id TEXT);",
			want: []string{
				"-- financial-tracker's own schema\nCREATE TABLE a (id TEXT);",
				"\nCREATE TABLE b (id TEXT);",
			},
		},
		{
			name:   "doubled single quote is an escaped literal quote, not a close+reopen",
			script: "INSERT INTO t (v) VALUES ('O''Reilly; still one value');\nSELECT 1;",
			want: []string{
				"INSERT INTO t (v) VALUES ('O''Reilly; still one value');",
				"\nSELECT 1;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStatements(tt.script)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitStatements(%q) = %#v, want %#v", tt.script, got, tt.want)
			}
		})
	}
}
