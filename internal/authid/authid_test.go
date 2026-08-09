package authid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateRejectsNonCanonicalIDs(t *testing.T) {
	tooLongSegment := strings.Repeat("界", maxSegmentBytes/3) + "a"
	oneByteOverID := makeByteSizedID(maxIDBytes+1, false)
	oneByteOverMultibyteID := makeByteSizedID(maxIDBytes+1, true)
	if len(tooLongSegment) != maxSegmentBytes+1 ||
		len(oneByteOverID) != maxIDBytes+1 ||
		len(oneByteOverMultibyteID) != maxIDBytes+1 {
		t.Fatal("byte-limit test setup is invalid")
	}
	tests := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "dot", id: "."},
		{name: "dot dot", id: ".."},
		{name: "nested dot", id: "a/./b.json"},
		{name: "nested dot dot", id: "a/../b.json"},
		{name: "leading slash", id: "/abs.json"},
		{name: "trailing slash", id: "a/"},
		{name: "empty segment", id: "a//b.json"},
		{name: "backslash", id: `a\b.json`},
		{name: "drive prefix", id: `C:token.json`},
		{name: "nested drive prefix", id: `a/C:token.json`},
		{name: "alternate data stream", id: "token.json:hidden"},
		{name: "reserved device", id: "NUL.json"},
		{name: "reserved numbered device", id: "provider/COM1.JSON"},
		{name: "reserved console input device", id: "CONIN$.json"},
		{name: "reserved superscript device", id: "provider/LPT².json"},
		{name: "reserved device with pre-extension space", id: "CON .json"},
		{name: "reserved numbered device with pre-extension space", id: "LPT1 .json"},
		{name: "trailing dot", id: "token.json."},
		{name: "trailing space", id: "token.json "},
		{name: "Windows wildcard", id: "token*.json"},
		{name: "NUL", id: "a\x00b.json"},
		{name: "newline", id: "a\nb.json"},
		{name: "DEL", id: "a\x7fb.json"},
		{name: "C1 control", id: "a\x85b.json"},
		{name: "bidi override", id: "token\u202ejson"},
		{name: "zero-width joiner", id: "token\u200d.json"},
		{name: "invalid UTF-8", id: string([]byte{'a', 0xff, 'b'})},
		{name: "oversized segment", id: tooLongSegment},
		{name: "oversized ID", id: oneByteOverID},
		{name: "oversized multibyte ID", id: oneByteOverMultibyteID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.id); err == nil {
				t.Fatalf("Validate(%q) unexpectedly succeeded", tc.id)
			}
		})
	}
}

func TestFoldKeyDetectsPortableIdentityCollisions(t *testing.T) {
	// Full Unicode folding deliberately over-approximates real filesystem
	// collisions (for example, ß and ss) so every accepted pair is portable.
	pairs := [][2]string{
		{"Token.json", "token.json"},
		{"café.json", "cafe\u0301.json"},
		{"provider/Straße.json", "PROVIDER/STRASSE.JSON"},
		{"α\u0345\u0342.json", "Α\u0345\u0342.json"},
	}
	for _, pair := range pairs {
		left, err := FoldKey(pair[0])
		if err != nil {
			t.Fatalf("FoldKey(%q): %v", pair[0], err)
		}
		right, err := FoldKey(pair[1])
		if err != nil {
			t.Fatalf("FoldKey(%q): %v", pair[1], err)
		}
		if left != right {
			t.Fatalf("FoldKey collision missed: %q = %q, %q = %q", pair[0], left, pair[1], right)
		}
	}
	if _, err := FoldKey("../token.json"); err == nil {
		t.Fatal("FoldKey accepted an invalid auth ID")
	}
}

func TestFoldKeyKeepsDistinctIDsDistinct(t *testing.T) {
	pairs := [][2]string{
		{"a/b.json", "ab.json"},
		{"provider/a.json", "provider2/a.json"},
		{"alpha.json", "beta.json"},
	}
	for _, pair := range pairs {
		left, err := FoldKey(pair[0])
		if err != nil {
			t.Fatalf("FoldKey(%q): %v", pair[0], err)
		}
		right, err := FoldKey(pair[1])
		if err != nil {
			t.Fatalf("FoldKey(%q): %v", pair[1], err)
		}
		if left == right {
			t.Fatalf("FoldKey collapsed distinct IDs %q and %q to %q", pair[0], pair[1], left)
		}
	}
}

func TestValidateAcceptsExactByteLimits(t *testing.T) {
	segment := strings.Repeat("a", maxSegmentBytes)
	if err := Validate(segment); err != nil {
		t.Fatalf("exact segment limit rejected: %v", err)
	}
	multibyteSegment := strings.Repeat("界", maxSegmentBytes/3)
	if len(multibyteSegment) != maxSegmentBytes {
		t.Fatalf("test setup produced %d-byte multibyte segment", len(multibyteSegment))
	}
	if err := Validate(multibyteSegment); err != nil {
		t.Fatalf("exact multibyte segment limit rejected: %v", err)
	}
	id := makeByteSizedID(maxIDBytes, false)
	if len(id) != maxIDBytes {
		t.Fatalf("test setup produced %d-byte ID", len(id))
	}
	if err := Validate(id); err != nil {
		t.Fatalf("exact ID limit rejected: %v", err)
	}
	multibyteID := makeByteSizedID(maxIDBytes, true)
	if len(multibyteID) != maxIDBytes || utf8.RuneCountInString(multibyteID) >= maxIDBytes {
		t.Fatalf("test setup produced %d-byte/%d-rune multibyte ID", len(multibyteID), utf8.RuneCountInString(multibyteID))
	}
	if err := Validate(multibyteID); err != nil {
		t.Fatalf("exact multibyte ID limit rejected: %v", err)
	}
}

func TestValidateAcceptsNestedAndOrdinaryIDs(t *testing.T) {
	for _, id := range []string{
		"token.json",
		"provider/tenant/user.json",
		"a/b/c/d.json",
		"..foo.json",
		"foo..bar.json",
		".hidden.json",
		"name with spaces.json",
		"user+tag@example.com.json",
		"café.json",
		"cafe\u0301.json",
		"Token.json",
		"token.json",
		"console.json",
		"NULL.json",
		"CONIN.json",
		"COM0.json",
		"communication/token.json",
		"fullwidth／solidus.json",
	} {
		t.Run(id, func(t *testing.T) {
			if err := Validate(id); err != nil {
				t.Fatalf("Validate(%q) error = %v", id, err)
			}
		})
	}
}

func TestFromFSPathAndToFSPathRoundTripNestedID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "provider", "tenant", "user.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := FromFSPath(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if id != "provider/tenant/user.json" {
		t.Fatalf("FromFSPath() = %q", id)
	}
	got, err := ToFSPath(root, id)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ToFSPath() = %q, want %q", got, want)
	}
}

func TestFromFSPathRejectsOutsideAndSymlinkedParent(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "token.json")
	if err := os.WriteFile(outsideFile, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FromFSPath(root, outsideFile); err == nil {
		t.Fatal("outside path unexpectedly accepted")
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := FromFSPath(root, filepath.Join(link, "token.json")); err == nil {
		t.Fatal("path through escaping symlink unexpectedly accepted")
	}
}

func TestFromFSPathAcceptsSymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	linkParent := t.TempDir()
	rootLink := filepath.Join(linkParent, "auths")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	path := filepath.Join(realRoot, "token.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := FromFSPath(rootLink, path)
	if err != nil {
		t.Fatal(err)
	}
	if id != "token.json" {
		t.Fatalf("FromFSPath() = %q", id)
	}
}

func TestFromFSPathAcceptsRelativeAndMissingParents(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "provider/token.json", want: "provider/token.json"},
		{path: "provider/tenant/new/token.json", want: "provider/tenant/new/token.json"},
		{path: "provider/../token.json", want: "token.json"},
		{path: filepath.Join(root, "absolute", "missing", "token.json"), want: "absolute/missing/token.json"},
	} {
		id, err := FromFSPath(root, tc.path)
		if err != nil {
			t.Fatalf("FromFSPath(%q): %v", tc.path, err)
		}
		if id != tc.want {
			t.Fatalf("FromFSPath(%q) = %q, want %q", tc.path, id, tc.want)
		}
	}
}

func TestFromFSPathRejectsEscapingRelativeInputs(t *testing.T) {
	root := t.TempDir()
	if _, err := FromFSPath(root, root); err == nil {
		t.Fatal("FromFSPath accepted the auth root itself as an ID")
	}
	for _, path := range []string{
		"../outside/token.json",
		"provider/../../token.json",
	} {
		if _, err := FromFSPath(root, path); err == nil {
			t.Fatalf("FromFSPath(%q) unexpectedly succeeded", path)
		}
	}
}

func TestFromFSPathRejectsMissingPathBelowEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := FromFSPath(root, filepath.Join(link, "missing", "token.json")); err == nil {
		t.Fatal("missing path below escaping symlink unexpectedly accepted")
	}
}

func TestFromFSPathRejectsNonDirectoryParent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.json")
	if err := os.WriteFile(file, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FromFSPath(root, filepath.Join(file, "nested", "token.json")); err == nil {
		t.Fatal("path below a regular file unexpectedly accepted")
	}
}

func TestFromFSPathCanonicalizesInRootParentSymlink(t *testing.T) {
	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realParent, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	id, err := FromFSPath(root, filepath.Join(root, "alias", "token.json"))
	if err != nil {
		t.Fatal(err)
	}
	if id != "real/token.json" {
		t.Fatalf("FromFSPath() = %q", id)
	}
	if _, err := ToFSPath(root, "alias/token.json"); err == nil {
		t.Fatal("ToFSPath accepted a noncanonical in-root symlink alias")
	}
	if _, err := ToFSPath(root, id); err != nil {
		t.Fatalf("ToFSPath rejected canonical target ID %q: %v", id, err)
	}
}

func TestToFSPathRejectsEscapingIntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ToFSPath(root, "alias/token.json"); err == nil {
		t.Fatal("ToFSPath accepted an ID below an escaping symlink")
	}
}

func TestToFSPathPreservesRootWhitespace(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "auths ")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ToFSPath(root, "token.json")
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(real, "token.json") {
		t.Fatalf("ToFSPath() = %q", got)
	}
}

func TestFromFSPathKeepsFinalSymlinkAsID(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	id, err := FromFSPath(root, link)
	if err != nil {
		t.Fatal(err)
	}
	if id != "link.json" {
		t.Fatalf("FromFSPath() = %q", id)
	}
	got, err := ToFSPath(root, id)
	if err != nil {
		t.Fatal(err)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(rootReal, "link.json") {
		t.Fatalf("ToFSPath() followed final symlink: got %q, target %q", got, target)
	}
}

func TestToFSPathRejectsHostileIDs(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{
		"",
		".",
		"..",
		"../token.json",
		"provider/../../token.json",
		"/absolute/token.json",
		`provider\token.json`,
		`C:token.json`,
	} {
		t.Run(id, func(t *testing.T) {
			if _, err := ToFSPath(root, id); err == nil {
				t.Fatalf("ToFSPath(%q) unexpectedly succeeded", id)
			}
		})
	}
}

func TestContainedRelativeRejectsOutsidePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := containedRelative(root, outside); err == nil {
		t.Fatal("outside path unexpectedly accepted")
	}
	if rel, err := containedRelative(root, filepath.Join(root, "nested", "token.json")); err != nil || rel != filepath.Join("nested", "token.json") {
		t.Fatalf("containedRelative() = %q, %v", rel, err)
	}
}

func TestFromFSPathRejectsSiblingPrefix(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "auth")
	sibling := filepath.Join(parent, "auths")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := FromFSPath(root, filepath.Join(sibling, "token.json")); err == nil {
		t.Fatal("sibling path sharing the root prefix unexpectedly accepted")
	}
}

func FuzzFromFSPathContained(f *testing.F) {
	root := f.TempDir()
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{"token.json", "a/b.json", "../x", `a\b`, "", "café.json"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, candidate string) {
		id, err := FromFSPath(root, candidate)
		if err != nil {
			return
		}
		if err := Validate(id); err != nil {
			t.Fatalf("FromFSPath returned invalid ID %q: %v", id, err)
		}
		path, err := ToFSPath(root, id)
		if err != nil {
			t.Fatalf("valid ID failed ToFSPath: %v", err)
		}
		rel, err := filepath.Rel(rootReal, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			t.Fatalf("valid ID escaped root: id=%q path=%q rel=%q err=%v", id, path, rel, err)
		}
		roundTrip, err := FromFSPath(root, path)
		if err != nil || roundTrip != id {
			t.Fatalf("round trip = %q, %v; want %q", roundTrip, err, id)
		}
	})
}

func makeByteSizedID(total int, multibyte bool) string {
	const segmentBytes = 200
	if maxSegmentBytes <= segmentBytes {
		panic("ID-limit fixture segments exceed maxSegmentBytes")
	}
	var result strings.Builder
	for result.Len() < total {
		if result.Len() > 0 {
			result.WriteByte('/')
		}
		remaining := total - result.Len()
		width := min(segmentBytes, remaining)
		if multibyte {
			result.WriteString(strings.Repeat("界", width/3))
			result.WriteString(strings.Repeat("a", width%3))
		} else {
			result.WriteString(strings.Repeat("a", width))
		}
	}
	return result.String()
}
