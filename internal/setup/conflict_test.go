package setup_test

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	. "gopkg.in/check.v1"

	"github.com/canonical/chisel/internal/setup"
	"github.com/canonical/chisel/internal/strdist"
)

func (s *S) TestPathToSegments(c *C) {
	tests := []struct {
		path     string
		segments []setup.PathSegment
		err      string
	}{{
		path: "/foo/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo/"},
			{Text: "bar"},
		},
	}, {
		path: "/foo/",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo/"},
			{Text: ""},
		},
	}, {
		path: "/",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: ""},
		},
	}, {
		path: "/*",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "*", HasGlob: true},
		},
	}, {
		path: "/*/",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "*/", HasGlob: true},
			{Text: ""},
		},
	}, {
		path: "/**",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "**", HasGlob: true, HasDoubleGlob: true},
		},
	}, {
		path: "/**/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "**/bar", HasGlob: true, HasDoubleGlob: true},
		},
	}, {
		path: "/foo*/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo*/", HasGlob: true},
			{Text: "bar"},
		},
	}, {
		path: "/foo?/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo?/", HasGlob: true},
			{Text: "bar"},
		},
	}, {
		path: "/fo??/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "fo??/", HasGlob: true},
			{Text: "bar"},
		},
	}, {
		path: "/f*o?/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "f*o?/", HasGlob: true},
			{Text: "bar"},
		},
	}, {
		path: "/f*oo/f**/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "f*oo/", HasGlob: true},
			{Text: "f**/bar", HasGlob: true, HasDoubleGlob: true},
		},
	}, {
		path: "/foo**/bar/baz",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo**/bar/baz", HasGlob: true, HasDoubleGlob: true},
		},
	}, {
		path: "/foo/**/sub/**/bar",
		segments: []setup.PathSegment{
			{Text: "/"},
			{Text: "foo/"},
			{Text: "**/sub/**/bar", HasGlob: true, HasDoubleGlob: true},
		},
	}, {
		path: "foo/bar",
		err:  `internal error: path does not start with '/'`,
	}}

	for _, test := range tests {
		c.Logf("Test: %q", test.path)
		segments, err := setup.PathToSegments(test.path)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
			continue
		}
		c.Assert(err, IsNil)
		c.Assert(segments, DeepEquals, test.segments)
	}
}

func (s *S) TestConflictTree(c *C) {
	sliceOne := &setup.Slice{
		Package: "pkg1",
		Name:    "path",
		Contents: map[string]setup.PathInfo{
			"/a/*/b": {Kind: setup.GlobPath},
		},
	}
	sliceTwo := &setup.Slice{
		Package: "pkg2",
		Name:    "glob",
		Contents: map[string]setup.PathInfo{
			"/a/*": {Kind: setup.GlobPath},
		},
	}

	pathOne := setup.PathSegmentSlice{
		Slice:     sliceOne,
		PathInfo:  setup.PathInfo{Kind: setup.GlobPath},
		WholePath: "/a/*/b",
	}
	pathTwo := setup.PathSegmentSlice{
		Slice:     sliceTwo,
		PathInfo:  setup.PathInfo{Kind: setup.GlobPath},
		WholePath: "/a/*",
	}

	tree := setup.NewConflictTree(map[string][]*setup.Slice{
		"/a/*/b": {sliceOne},
		"/a/*":   {sliceTwo},
	})
	err := tree.HasConflict()
	c.Assert(err, IsNil)

	expected := &setup.PathNode{
		Segment: setup.PathSegment{Text: "/"},
		Children: map[string]*setup.PathNode{
			"a/": {
				Segment:       setup.PathSegment{Text: "a/"},
				SegmentSlices: []*setup.PathSegmentSlice{&pathOne, &pathTwo},
				Children: map[string]*setup.PathNode{
					"*": {
						Segment:       setup.PathSegment{Text: "*", HasGlob: true},
						SegmentSlices: []*setup.PathSegmentSlice{&pathTwo},
					},
					"*/": {
						Segment:       setup.PathSegment{Text: "*/", HasGlob: true},
						SegmentSlices: []*setup.PathSegmentSlice{&pathOne},
						Children: map[string]*setup.PathNode{
							"b": {
								Segment:       setup.PathSegment{Text: "b"},
								SegmentSlices: []*setup.PathSegmentSlice{&pathOne},
							},
						},
					},
				},
			},
		},
	}
	assertTreeEquals(c, tree.Root, expected)
}

func assertTreeEquals(c *C, obtained, expected *setup.PathNode) {
	c.Assert(obtained.Segment, DeepEquals, expected.Segment)

	slices.SortFunc(obtained.SegmentSlices, func(a, b *setup.PathSegmentSlice) int {
		return strings.Compare(a.Slice.String(), b.Slice.String())
	})
	slices.SortFunc(expected.SegmentSlices, func(a, b *setup.PathSegmentSlice) int {
		return strings.Compare(a.Slice.String(), b.Slice.String())
	})
	c.Assert(obtained.SegmentSlices, DeepEquals, expected.SegmentSlices)

	c.Assert(len(obtained.Children), Equals, len(expected.Children))
	for name, expectedChild := range expected.Children {
		obtainedChild, ok := obtained.Children[name]
		c.Assert(ok, Equals, true)
		assertTreeEquals(c, obtainedChild, expectedChild)
	}
}

// FuzzPathToSegments checks structural invariants of the segments produced
// for an arbitrary path: they join back to the original path, the glob flags
// reflect the segment content, and only the final segment may terminate the
// path (empty segment for directories, no trailing "/" otherwise).
func FuzzPathToSegments(f *testing.F) {
	for _, seed := range []string{
		"/", "/a", "/a/", "/a/b", "/a//b", "/*", "/**", "/a/**/", "/a/**/b",
		"/f*o?/bar", "/foo**/bar/baz", "/foo/**/sub/**/bar", "", "foo/bar",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, fuzzPath string) {
		segments, err := setup.PathToSegments(fuzzPath)
		if !strings.HasPrefix(fuzzPath, "/") {
			if err == nil {
				t.Fatalf("path %q: expected error", fuzzPath)
			}
			return
		}
		if err != nil {
			t.Fatalf("path %q: unexpected error: %v", fuzzPath, err)
		}
		if len(segments) < 2 {
			t.Fatalf("path %q: expected at least two segments, got %#v", fuzzPath, segments)
		}
		if segments[0] != (setup.PathSegment{Text: "/"}) {
			t.Fatalf("path %q: first segment %#v is not the root segment", fuzzPath, segments[0])
		}
		var joined strings.Builder
		for i, seg := range segments {
			joined.WriteString(seg.Text)
			if seg.HasGlob != strings.ContainsAny(seg.Text, "*?") {
				t.Fatalf("path %q: segment %q has inconsistent HasGlob %v", fuzzPath, seg.Text, seg.HasGlob)
			}
			if seg.HasDoubleGlob != strings.Contains(seg.Text, "**") {
				t.Fatalf("path %q: segment %q has inconsistent HasDoubleGlob %v", fuzzPath, seg.Text, seg.HasDoubleGlob)
			}
			last := i == len(segments)-1
			if !last && !strings.HasSuffix(seg.Text, "/") {
				t.Fatalf("path %q: non-final segment %q does not end with '/'", fuzzPath, seg.Text)
			}
			if last && seg.Text != "" && strings.HasSuffix(seg.Text, "/") {
				t.Fatalf("path %q: final segment %q is neither empty nor free of a trailing '/'", fuzzPath, seg.Text)
			}
			if !seg.HasDoubleGlob && strings.Contains(strings.TrimSuffix(seg.Text, "/"), "/") {
				t.Fatalf("path %q: segment %q without '**' contains an interior '/'", fuzzPath, seg.Text)
			}
		}
		if joined.String() != fuzzPath {
			t.Fatalf("path %q: segments %#v join back to %q", fuzzPath, segments, joined.String())
		}
	})
}

// FuzzConflictTree compares the tree-based conflict detection against a
// reference implementation that checks every pair of whole paths with
// strdist.GlobPath, using the same package and path kind rules.
func FuzzConflictTree(f *testing.F) {
	f.Add("/a/*", "/a/*/b", "/a/bar/b", uint8(0b010))
	f.Add("/path/**", "/path/subdir/**", "/path/subdir/file", uint8(0b011))
	f.Add("/a/", "/a/*", "/a/b", uint8(0b000))
	f.Add("/path/subdir/**", "/path/subdir/f*", "/other", uint8(0b001_001))
	f.Add("/f*o?/bar", "/foob/bar", "/foo/bar/", uint8(0b110))
	f.Fuzz(func(t *testing.T, pathA, pathB, pathC string, bits uint8) {
		type conflictInput struct {
			path string
			kind setup.PathKind
			pkg  string
		}
		var inputs []conflictInput
		seen := map[string]bool{}
		for i, fuzzPath := range []string{pathA, pathB, pathC} {
			if !isSupportedFuzzPath(fuzzPath) || seen[fuzzPath] {
				continue
			}
			seen[fuzzPath] = true
			pkg := "pkg1"
			if bits&(1<<i) != 0 {
				pkg = "pkg2"
			}
			kind := derivePathKind(fuzzPath, bits&(1<<(i+3)) != 0)
			inputs = append(inputs, conflictInput{fuzzPath, kind, pkg})
		}
		if len(inputs) < 2 {
			t.Skip("fewer than two valid distinct paths")
		}

		// Reference implementation: check every pair of whole paths.
		globOrCopy := func(kind setup.PathKind) bool {
			return kind == setup.GlobPath || kind == setup.CopyPath
		}
		wantConflict := false
		for i := 0; i < len(inputs) && !wantConflict; i++ {
			for j := i + 1; j < len(inputs); j++ {
				a, b := inputs[i], inputs[j]
				if a.pkg == b.pkg && globOrCopy(a.kind) && globOrCopy(b.kind) {
					// Content extracted from the same package cannot conflict.
					continue
				}
				if strdist.GlobPath(a.path, b.path) {
					wantConflict = true
					break
				}
			}
		}

		pathToSlices := map[string][]*setup.Slice{}
		for i, input := range inputs {
			slice := &setup.Slice{
				Package:  input.pkg,
				Name:     fmt.Sprintf("slice%d", i),
				Contents: map[string]setup.PathInfo{input.path: {Kind: input.kind}},
			}
			pathToSlices[input.path] = []*setup.Slice{slice}
		}
		tree := setup.NewConflictTree(pathToSlices)
		err := tree.HasConflict()
		if gotConflict := err != nil; gotConflict != wantConflict {
			t.Fatalf("conflict mismatch: tree=%v reference=%v (err=%v)\ninputs: %+v",
				gotConflict, wantConflict, err, inputs)
		}
	})
}

// isValidContentPath mirrors the content path validation performed when
// parsing slice definitions, see yaml.go.
func isValidContentPath(contPath string) bool {
	if contPath == "" {
		return false
	}
	comparePath := strings.TrimSuffix(contPath, "/")
	return path.IsAbs(contPath) && path.Clean(contPath) == comparePath
}

// isSupportedFuzzPath restricts the differential fuzzing to paths where the
// whole-path strdist.GlobPath reference is trustworthy. Besides the content
// path validation from yaml.go it excludes:
//
//   - Invalid utf-8. It cannot reach conflict detection because the yaml
//     parser rejects it, and it trips an inconsistency in strdist.GlobPath
//     itself: the prefix/suffix fast paths compare bytes while the distance
//     computation compares runes, and all invalid bytes decode to the same
//     replacement rune.
//   - The rune U+2051 (two asterisks aligned vertically), which
//     strdist.GlobPath uses internally as the replacement token for "**" and
//     therefore treats as a wildcard even in otherwise literal paths. For
//     such paths the tree genuinely diverges from whole-path GlobPath:
//     literal segments are compared with ==, which treats U+2051 as the
//     literal character.
func isSupportedFuzzPath(contPath string) bool {
	return utf8.ValidString(contPath) &&
		!strings.ContainsRune(contPath, '\u2051') &&
		isValidContentPath(contPath)
}

// isValidGeneratePath mirrors validateGeneratePath in yaml.go.
func isValidGeneratePath(contPath string) bool {
	if !strings.HasSuffix(contPath, "/**") {
		return false
	}
	dirPath := strings.TrimSuffix(contPath, "**")
	return !strings.ContainsAny(dirPath, "*?")
}

// derivePathKind returns a path kind that yaml parsing could assign to the
// path: wildcard paths are GlobPath, or GeneratePath when generate is set and
// the path allows it. Directory-shaped paths are taken to have make:true.
func derivePathKind(contPath string, generate bool) setup.PathKind {
	switch {
	case strings.ContainsAny(contPath, "*?"):
		if generate && isValidGeneratePath(contPath) {
			return setup.GeneratePath
		}
		return setup.GlobPath
	case strings.HasSuffix(contPath, "/"):
		return setup.DirPath
	default:
		return setup.CopyPath
	}
}

// FuzzConflictTreeMany extends FuzzConflictTree to an arbitrary number of
// paths (one per line of the blob argument, capped at 8) with several slices
// allowed to share a path, per-slice path kinds on wildcard-free paths, and
// additionally checks the structure of the built tree when no conflict is
// found.
func FuzzConflictTreeMany(f *testing.F) {
	f.Add("/a/*\n/a/*/b\n/a/bar/b\n/a/\n/a/b", uint16(0b10101), uint16(0), uint16(0))
	f.Add("/path/**\n/path/subdir/**\n/path/subdir/f*\n/path/subdir/file", uint16(0b0110), uint16(0b0011), uint16(0))
	f.Add("/a/b\n/a/b\n/a/*\n/a/*\n/a/", uint16(0b01010), uint16(0), uint16(0b01_00_00_00_01))
	f.Add("/a/**/\n/a/b/\n/a/b/c\n/a/**", uint16(0b1001), uint16(0b1000), uint16(0))
	f.Add("/etc/foo/\n/etc/foo/**\n/etc/foo/bar", uint16(0), uint16(0), uint16(0))
	f.Fuzz(func(t *testing.T, blob string, pkgBits uint16, genBits uint16, kindBits uint16) {
		const maxPaths = 8
		wildcardKinds := map[string]setup.PathKind{}
		seenPath := map[string]bool{}
		pathToSlices := map[string][]*setup.Slice{}
		var paths []string
		i := 0
		for _, fuzzPath := range strings.Split(blob, "\n") {
			if i >= maxPaths {
				break
			}
			if !isSupportedFuzzPath(fuzzPath) {
				continue
			}
			pkg := "pkg1"
			if pkgBits&(1<<i) != 0 {
				pkg = "pkg2"
			}
			var kind setup.PathKind
			if strings.ContainsAny(fuzzPath, "*?") {
				// Wildcard paths cannot use "prefer", so same-path contents
				// must be equal (validated before conflict detection runs,
				// see Release.validate) and the kind is fixed by the first
				// slice that has the path.
				var ok bool
				kind, ok = wildcardKinds[fuzzPath]
				if !ok {
					kind = derivePathKind(fuzzPath, genBits&(1<<i) != 0)
					wildcardKinds[fuzzPath] = kind
				}
			} else if strings.HasSuffix(fuzzPath, "/") {
				// A directory-shaped entry is CopyPath unless it has
				// make:true. Through "prefer", slices sharing a path may
				// disagree on the kind, so it is chosen per slice.
				kind = setup.CopyPath
				if kindBits>>(2*i)&1 != 0 {
					kind = setup.DirPath
				}
			} else {
				switch kindBits >> (2 * i) & 3 {
				case 1:
					kind = setup.TextPath
				case 2:
					kind = setup.SymlinkPath
				default:
					kind = setup.CopyPath
				}
			}
			if !seenPath[fuzzPath] {
				seenPath[fuzzPath] = true
				paths = append(paths, fuzzPath)
			}
			slice := &setup.Slice{
				Package:  pkg,
				Name:     fmt.Sprintf("slice%d", i),
				Contents: map[string]setup.PathInfo{fuzzPath: {Kind: kind}},
			}
			pathToSlices[fuzzPath] = append(pathToSlices[fuzzPath], slice)
			i++
		}
		if len(paths) < 2 {
			t.Skip("fewer than two valid distinct paths")
		}

		// Reference implementation: check every slice pair on distinct
		// paths. Slices sharing a path are never compared here because
		// identical paths are validated separately before conflict detection
		// runs.
		globOrCopy := func(kind setup.PathKind) bool {
			return kind == setup.GlobPath || kind == setup.CopyPath
		}
		wantConflict := false
		for i := 0; i < len(paths) && !wantConflict; i++ {
			for j := i + 1; j < len(paths) && !wantConflict; j++ {
				if !strdist.GlobPath(paths[i], paths[j]) {
					continue
				}
				for _, a := range pathToSlices[paths[i]] {
					for _, b := range pathToSlices[paths[j]] {
						skip := a.Package == b.Package &&
							globOrCopy(a.Contents[paths[i]].Kind) &&
							globOrCopy(b.Contents[paths[j]].Kind)
						if !skip {
							wantConflict = true
						}
					}
				}
			}
		}

		tree := setup.NewConflictTree(pathToSlices)
		err := tree.HasConflict()
		if gotConflict := err != nil; gotConflict != wantConflict {
			t.Fatalf("conflict mismatch: tree=%v reference=%v (err=%v)\npaths: %q",
				gotConflict, wantConflict, err, paths)
		}
		if err == nil {
			// On conflict the tree is left partially built, but on success
			// it must hold every path.
			assertTreeInvariants(t, tree, pathToSlices)
		}
	})
}

// assertTreeInvariants checks the structure of a fully built conflict tree:
// every inserted path is reachable through its segment chain with each of
// its slices present on every node of the chain, the final segment's node is
// childless, and every node is consistent with its segment text.
func assertTreeInvariants(t *testing.T, tree setup.PathConflictTree, pathToSlices map[string][]*setup.Slice) {
	for fuzzPath, pathSlices := range pathToSlices {
		segments, err := setup.PathToSegments(fuzzPath)
		if err != nil {
			t.Fatalf("path %q: %v", fuzzPath, err)
		}
		node := tree.Root
		if node.Segment != segments[0] {
			t.Fatalf("root segment is %#v", node.Segment)
		}
		for _, seg := range segments[1:] {
			child, ok := node.Children[seg.Text]
			if !ok {
				t.Fatalf("path %q: no node for segment %q", fuzzPath, seg.Text)
			}
			if child.Segment != seg {
				t.Fatalf("path %q: node segment %#v differs from %#v", fuzzPath, child.Segment, seg)
			}
			for _, slice := range pathSlices {
				found := slices.ContainsFunc(child.SegmentSlices, func(ss *setup.PathSegmentSlice) bool {
					return ss.Slice == slice && ss.WholePath == fuzzPath
				})
				if !found {
					t.Fatalf("path %q: slice %s missing on node %q", fuzzPath, slice, seg.Text)
				}
			}
			node = child
		}
		if len(node.Children) > 0 {
			t.Fatalf("path %q: final node %q has children", fuzzPath, node.Segment.Text)
		}
	}
	assertNodeInvariants(t, tree.Root)
}

func assertNodeInvariants(t *testing.T, node *setup.PathNode) {
	text := node.Segment.Text
	if node.Segment.HasGlob != strings.ContainsAny(text, "*?") {
		t.Fatalf("node %q has inconsistent HasGlob %v", text, node.Segment.HasGlob)
	}
	if node.Segment.HasDoubleGlob != strings.Contains(text, "**") {
		t.Fatalf("node %q has inconsistent HasDoubleGlob %v", text, node.Segment.HasDoubleGlob)
	}
	for _, child := range node.Children {
		if !strings.HasSuffix(text, "/") {
			t.Fatalf("node %q has children but no trailing '/'", text)
		}
		if len(child.SegmentSlices) == 0 {
			t.Fatalf("node %q has no slices", child.Segment.Text)
		}
		assertNodeInvariants(t, child)
	}
}

// benchmarkPaths returns a conflict-free path map shaped like a real release:
// most paths share long literal prefixes, each package owns globs under its
// own directories, and /usr/bin holds one glob per package in a shared
// directory.
func benchmarkPaths(numPkgs int) map[string][]*setup.Slice {
	paths := make(map[string][]*setup.Slice)
	for i := 0; i < numPkgs; i++ {
		pkgName := fmt.Sprintf("pkg%04d", i)
		slice := &setup.Slice{
			Package:  pkgName,
			Name:     "bench",
			Contents: map[string]setup.PathInfo{},
		}
		add := func(contPath string, kind setup.PathKind) {
			slice.Contents[contPath] = setup.PathInfo{Kind: kind}
			paths[contPath] = append(paths[contPath], slice)
		}
		add("/usr/lib/x86_64-linux-gnu/"+pkgName+"/lib"+pkgName+".so.1", setup.CopyPath)
		add("/usr/lib/x86_64-linux-gnu/"+pkgName+"/*.so*", setup.GlobPath)
		add("/usr/share/doc/"+pkgName+"/copyright", setup.CopyPath)
		add("/usr/bin/"+pkgName+"-*", setup.GlobPath)
		add("/etc/"+pkgName+"/", setup.DirPath)
		add("/var/lib/"+pkgName+"/**", setup.GlobPath)
	}
	return paths
}

func BenchmarkConflictTree(b *testing.B) {
	for _, numPkgs := range []int{10, 100, 1000} {
		paths := benchmarkPaths(numPkgs)
		b.Run(fmt.Sprintf("pkgs=%d", numPkgs), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				tree := setup.NewConflictTree(paths)
				if err := tree.HasConflict(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPathToSegments(b *testing.B) {
	paths := []string{
		"/usr/lib/x86_64-linux-gnu/libssl.so.3",
		"/usr/share/doc/openssl/",
		"/usr/bin/openssl-*",
		"/etc/ssl/certs/**",
		"/usr/lib/**/engines-3/*.so",
	}
	for i := 0; i < b.N; i++ {
		for _, p := range paths {
			if _, err := setup.PathToSegments(p); err != nil {
				b.Fatal(err)
			}
		}
	}
}
