package mask

import "testing"

// Synthetic fixture tokens, split into concatenated fragments: the runtime
// value is identical, but the literal never appears contiguously in source,
// so GitHub push protection does not mistake the fixtures for real secrets.
const (
	ghpClassic = "ghp_" + "0123456789abcdefABCD"
	ghpFine    = "github_pat_" + "11AaBbCc0123456789XYZx"
	awsKey     = "AKIA" + "IOSFODNN7EXAMPLE"
	googleKey  = "AIza" + "SyA1b2C3d4E5f6g7H8i9J0k1L2m3N4o5P6q"
	slackToken = "xoxb-" + "123456789012-1234567890123-abcdefghijklmnopqrstuv"
)

func TestMaskTokenFamilies(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "github classic token",
			in:   "token " + ghpClassic + " commit",
			want: "token ghp_…ABCD commit",
		},
		{
			name: "github fine-grained token",
			in:   ghpFine,
			want: "github_pat_…XYZx",
		},
		{
			name: "openai key",
			in:   "export OPENAI_API_KEY=sk-proj-aaaa1111bbbb2222cccc",
			want: "export OPENAI_API_KEY=sk-p…cccc",
		},
		{
			name: "anthropic key",
			in:   "ANTHROPIC key sk-ant-api03-0123456789abcdef",
			want: "ANTHROPIC key sk-a…cdef",
		},
		{
			name: "jwt",
			in:   "Authorization: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJVadQssw5c",
			want: "Authorization: eyJh…sw5c",
		},
		{
			name: "aws access key",
			in:   "aws " + awsKey + " config",
			want: "aws AKIA…MPLE config",
		},
		{
			name: "google api key",
			in:   "key = " + googleKey,
			want: "key = AIza…5P6q",
		},
		{
			name: "slack token",
			in:   "SLACK=" + slackToken,
			want: "SLACK=xoxb…stuv",
		},
		{
			name: "bearer header",
			in:   "Authorization: Bearer abcdef1234567890abcdef",
			want: "Authorization: Bear…cdef",
		},
		{
			name: "url password",
			in:   "postgres://user:hunter2secret@db.example.com/prod",
			want: "postgres://user:***@db.example.com/prod",
		},
		{
			name: "query param token",
			in:   "curl https://api.example.com/v1?token=abcdef0123456789&x=1",
			want: "curl https://api.example.com/v1?token=…6789&x=1",
		},
		{
			name: "query param api key case-insensitive",
			in:   "https://api.example.com/v1?API_KEY=zzzz9999zzzz8888",
			want: "https://api.example.com/v1?API_KEY=…8888",
		},
		{
			name: "two secrets on one line",
			in:   ghpClassic + " and sk-aaaa1111bbbb2222cccc",
			want: "ghp_…ABCD and sk-a…cccc",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Mask(c.in); got != c.want {
				t.Errorf("Mask(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMaskLeavesPlainText(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"plain sentence", "the quick brown fox jumps over the lazy dog"},
		{"short dash word", "sk-prototype"},
		{"base64 blob without key prefix", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo="},
		{"github word without token", "github pages action run"},
		{"bearer word alone", "bearer of bad news"},
		{"url without password", "https://example.com/path?a=b"},
		{"eyecatcher word", "eyewitness reports"},
		{"multiline stays intact", "line one\nline two\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Mask(c.in); got != c.in {
				t.Errorf("Mask(%q) = %q, want unchanged", c.in, got)
			}
		})
	}
}

func TestMaskIdempotent(t *testing.T) {
	once := Mask("key " + ghpClassic + " end")
	if twice := Mask(once); twice != once {
		t.Errorf("Mask not idempotent: %q then %q", once, twice)
	}
}
