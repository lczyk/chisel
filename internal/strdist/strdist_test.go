package strdist_test

import (
	. "gopkg.in/check.v1"

	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/canonical/chisel/internal/strdist"
)

type distanceTest struct {
	a, b string
	f    strdist.CostFunc
	r    int64
	cut  int64
}

func uniqueCost(ar, br rune) strdist.Cost {
	return strdist.Cost{SwapAB: 1, DeleteA: 3, InsertB: 5}
}

var distanceTests = []distanceTest{
	{f: uniqueCost, r: 0, a: "abc", b: "abc"},
	{f: uniqueCost, r: 1, a: "abc", b: "abd"},
	{f: uniqueCost, r: 1, a: "abc", b: "adc"},
	{f: uniqueCost, r: 1, a: "abc", b: "dbc"},
	{f: uniqueCost, r: 2, a: "abc", b: "add"},
	{f: uniqueCost, r: 2, a: "abc", b: "ddc"},
	{f: uniqueCost, r: 2, a: "abc", b: "dbd"},
	{f: uniqueCost, r: 3, a: "abc", b: "ddd"},
	{f: uniqueCost, r: 3, a: "abc", b: "ab"},
	{f: uniqueCost, r: 3, a: "abc", b: "bc"},
	{f: uniqueCost, r: 3, a: "abc", b: "ac"},
	{f: uniqueCost, r: 6, a: "abc", b: "a"},
	{f: uniqueCost, r: 6, a: "abc", b: "b"},
	{f: uniqueCost, r: 6, a: "abc", b: "c"},
	{f: uniqueCost, r: 9, a: "abc", b: ""},
	{f: uniqueCost, r: 6, cut: 6, a: "abc", b: ""},
	{f: uniqueCost, r: 5, a: "abc", b: "abcd"},
	{f: uniqueCost, r: 5, a: "abc", b: "dabc"},
	{f: uniqueCost, r: 10, a: "abc", b: "adbdc"},
	{f: uniqueCost, r: 10, a: "abc", b: "dabcd"},
	{f: uniqueCost, r: 40, a: "abc", b: "ddaddbddcdd"},
	{f: strdist.StandardCost, r: 3, a: "abcdefg", b: "axcdfgh"},
	{f: strdist.StandardCost, r: 2, cut: 2, a: "abcdef", b: "abc"},
	{f: strdist.StandardCost, r: 2, cut: 3, a: "abcdef", b: "abcd"},
	{f: strdist.StandardCost, r: 3, a: "abc", b: ""},
	{f: strdist.StandardCost, r: 1, cut: 1, a: "abc", b: ""},
	// Not symmetric.
	{f: strdist.StandardCost, r: 2, cut: 3, a: "ab", b: ""},
	{f: strdist.StandardCost, r: 2, cut: 1, a: "", b: "ab"},
	{f: strdist.GlobCost, r: 0, a: "abc*", b: "abcdef"},
	{f: strdist.GlobCost, r: 0, a: "ab*ef", b: "abcdef"},
	{f: strdist.GlobCost, r: 0, a: "*def", b: "abcdef"},
	{f: strdist.GlobCost, r: 0, a: "a*/def", b: "abc/def"},
	{f: strdist.GlobCost, r: 1, a: "a*/def", b: "abc/gef"},
	{f: strdist.GlobCost, r: 0, a: "a*/*f", b: "abc/def"},
	{f: strdist.GlobCost, r: 1, a: "a*/*f", b: "abc/defh"},
	{f: strdist.GlobCost, r: 1, a: "a*/*f", b: "abc/defhi"},
	{f: strdist.GlobCost, r: strdist.Inhibit, a: "a*", b: "abc/def"},
	{f: strdist.GlobCost, r: strdist.Inhibit, a: "a*/*f", b: "abc/def/hij"},
	{f: strdist.GlobCost, r: 0, a: "a**f/hij", b: "abc/def/hij"},
	{f: strdist.GlobCost, r: 1, a: "a**f/hij", b: "abc/def/hik"},
	{f: strdist.GlobCost, r: 2, a: "a**fg", b: "abc/def/hik"},
	{f: strdist.GlobCost, r: 0, a: "a**f/hij/klm", b: "abc/d**m"},
	{f: strdist.GlobCost, r: 1, a: "**a", b: ""},
	{f: strdist.GlobCost, r: 0, a: "/*a/", b: "/a/"},
	// Edge cases with empty strings.
	{f: strdist.GlobCost, r: strdist.Inhibit, cut: 1, a: "/", b: ""},
	{f: strdist.GlobCost, r: strdist.Inhibit, cut: 1, a: "", b: "/"},
	{f: strdist.GlobCost, r: 0, cut: 1, a: "*", b: ""},
	{f: strdist.GlobCost, r: 0, cut: 1, a: "", b: "*"},
	{f: strdist.GlobCost, r: 0, cut: 1, a: "**", b: ""},
	{f: strdist.GlobCost, r: 0, cut: 1, a: "", b: "**"},
}

func (s *S) TestDistance(c *C) {
	for _, test := range distanceTests {
		c.Logf("Test: %v", test)
		if strings.Contains(test.a, "*") || strings.Contains(test.b, "*") {
			c.Assert(strdist.GlobPath(test.a, test.b), Equals, test.r == 0)
		}
		f := test.f
		if f == nil {
			f = strdist.StandardCost
		}
		test.a = strings.ReplaceAll(test.a, "**", "⁑")
		test.b = strings.ReplaceAll(test.b, "**", "⁑")
		r := strdist.Distance(test.a, test.b, f, test.cut)
		c.Assert(r, Equals, test.r)
	}
}

func BenchmarkDistance(b *testing.B) {
	const one = "abdefghijklmnopqrstuvwxyz"
	const two = "a.d.f.h.j.l.n.p.r.t.v.x.z"
	for i := 0; i < b.N; i++ {
		strdist.Distance(one, two, strdist.StandardCost, 0)
	}
}

func BenchmarkDistanceCut(b *testing.B) {
	const one = "abdefghijklmnopqrstuvwxyz"
	const two = "a.d.f.h.j.l.n.p.r.t.v.x.z"
	for i := 0; i < b.N; i++ {
		strdist.Distance(one, two, strdist.StandardCost, 1)
	}
}

// FuzzGlobPathIntersection compares GlobPath against a reference answering
// "is there a string that matches both glob patterns". The reference does a
// BFS over the product of the two patterns' NFAs. Patterns are restricted to
// the alphabet {a, b, c, /} plus wildcards. Only the equality relation
// between literals matters to both implementations, and three distinct non-/
// literals cover every such relation with up to three classes; a witness
// never needs characters outside the alphabet because any character consumed
// by wildcards on both sides can be replaced with 'a'.
//
// When the reference finds an intersection it produces a witness string,
// which is independently checked against both patterns using the stdlib
// regexp package, so a bug in the NFA construction cannot silently produce
// spurious intersections.
func FuzzGlobPathIntersection(f *testing.F) {
	f.Add("/a/*", "/a/b")
	f.Add("/a/*", "/a/b/")
	f.Add("/**", "/a/b/a")
	f.Add("/*a/**", "/ab/a/")
	f.Add("/a?b", "/ab*")
	f.Add("/*ab", "/ba*")
	f.Add("/a/**/b", "/a/*/b")
	f.Add("/a**b/", "/a/b/")
	f.Add("/***", "/a*b")
	f.Add("*/", "/*")
	f.Fuzz(func(t *testing.T, a, b string) {
		const alphabet = "abc/"
		valid := func(s string) bool {
			if len(s) > 16 {
				return false
			}
			return !strings.ContainsFunc(s, func(r rune) bool {
				return !strings.ContainsRune(alphabet+"*?", r)
			})
		}
		if !valid(a) || !valid(b) {
			t.Skip("pattern outside the reference alphabet")
		}

		// Mirror the "**" handling in GlobPath: pairs of '*' collapse
		// left-to-right, a leftover single '*' stands on its own.
		ra := []rune(strings.ReplaceAll(a, "**", "\u2051"))
		rb := []rune(strings.ReplaceAll(b, "**", "\u2051"))

		witness, intersect := globsIntersect(ra, rb, alphabet)
		if intersect {
			for _, p := range [][]rune{ra, rb} {
				re := globToRegexp(p)
				if !re.MatchString(witness) {
					t.Fatalf("reference bug: witness %q does not match %q (%v)", witness, string(p), re)
				}
			}
		}

		if got := strdist.GlobPath(a, b); got != intersect {
			t.Fatalf("GlobPath(%q, %q) = %v, but reference says intersect = %v (witness %q)",
				a, b, got, intersect, witness)
		}
	})
}

// globsIntersect reports whether some string matches both glob patterns,
// and returns such a string if it exists. Patterns are rune slices where
// '\u2051' stands for "**". Only literals from the given alphabet are supported;
// the alphabet must not contain wildcards and must include '/' and at least
// one other character.
func globsIntersect(a, b []rune, alphabet string) (witness string, ok bool) {
	// closure returns the pattern positions reachable from i without
	// consuming a character, i.e. by skipping '*' and '\u2051' tokens.
	closure := func(p []rune, i int) []int {
		out := []int{i}
		for i < len(p) && (p[i] == '*' || p[i] == '\u2051') {
			i++
			out = append(out, i)
		}
		return out
	}
	accepts := func(p []rune, i int) bool {
		return slices.Contains(closure(p, i), len(p))
	}
	// next returns the pattern positions reachable from i by consuming c:
	// literals and '?' advance, '*' and '\u2051' consume and stay.
	next := func(p []rune, i int, c rune) []int {
		var out []int
		for _, k := range closure(p, i) {
			if k == len(p) {
				continue
			}
			switch p[k] {
			case '\u2051':
				out = append(out, k)
			case '*':
				if c != '/' {
					out = append(out, k)
				}
			case '?':
				if c != '/' {
					out = append(out, k+1)
				}
			default:
				if p[k] == c {
					out = append(out, k+1)
				}
			}
		}
		return out
	}

	type state struct{ i, j int }
	type visit struct {
		prev state
		c    rune
	}
	start := state{0, 0}
	visited := map[state]visit{start: {}}
	queue := []state{start}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if accepts(a, s.i) && accepts(b, s.j) {
			var chars []rune
			for s != start {
				v := visited[s]
				chars = append(chars, v.c)
				s = v.prev
			}
			slices.Reverse(chars)
			return string(chars), true
		}
		for _, c := range alphabet {
			for _, ni := range next(a, s.i, c) {
				for _, nj := range next(b, s.j, c) {
					ns := state{ni, nj}
					if _, ok := visited[ns]; !ok {
						visited[ns] = visit{s, c}
						queue = append(queue, ns)
					}
				}
			}
		}
	}
	return "", false
}

// globToRegexp converts a glob pattern (with '\u2051' standing for "**") to an
// equivalent anchored regular expression.
func globToRegexp(p []rune) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("\\A")
	for _, r := range p {
		switch r {
		case '\u2051':
			sb.WriteString("(?s:.*)")
		case '*':
			sb.WriteString("[^/]*")
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	sb.WriteString("\\z")
	return regexp.MustCompile(sb.String())
}
