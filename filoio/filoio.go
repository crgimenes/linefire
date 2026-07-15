// Package filoio reads and writes Linefire's documents in the Filo language.
//
// Filo is used as an S-expression DSL, not as JSON with parentheses: registered
// builtins build a tagged value tree, hand-written decoders turn that tree into the
// plain model structs, and Save emits Filo text by hand. That keeps `asset` and
// `level` free of any serialization concern — they stay pure models — and it leaves
// room for the reason this format was chosen in the first place: because a document is
// a PROGRAM, an author can compute a value inline (arithmetic, a generator such as a
// rectangle of wall) without a new struct field or a new case in the engine.
//
// Filo's own vocabulary is off limits: the core already defines 31 builtins (list, map,
// range, string, head, length, not, ...) and 15 special forms (def, set, do, if, ...).
// RegisterBuiltin refuses a duplicate, so a clash fails loudly at Load rather than
// silently shadowing. That is why the map document is (level ...) and not (map ...).
package filoio

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
)

// File extensions. The syntax is Filo, but the extension is app-specific (a plain
// .filo is any Filo program) so the OS can associate them and the editor's open dialog
// can filter by document type.
const (
	ExtAsset = ".lfa" // one drawable object: a ship, a weapon, a power-up
	ExtLevel = ".lfm" // one stage
)

// AssetPath resolves an asset REFERENCE — a bare name such as "turret", the way a level
// and the procedural generator write it — to a file inside dir. Documents name things;
// only the loader knows where they live and what they are called on disk. Three separate
// conventions for this had grown (relative to the cwd, relative to the map directory,
// hardcoded to "gameassets/"), which is why running the game from another working
// directory used to break spawns but not the horde.
func AssetPath(dir, ref string) string {
	if ref == "" {
		return ""
	}
	return filepath.Join(dir, ref+ExtAsset)
}

// LevelPath resolves a map name to its file inside dir.
func LevelPath(dir, name string) string {
	return filepath.Join(dir, name+ExtLevel)
}

// tagged wraps already-evaluated args in a list whose head is a tag string. Every
// builtin below is a tag constructor: validation is deferred to the decoders, so the
// error a user sees names the field, not the parser.
func tagged(tag string, items ...filo.Value) filo.Value {
	return filo.VList(append([]filo.Value{filo.VString(tag)}, items...))
}

// biTagged is the builtin behind a form that carries no logic of its own.
func biTagged(tag string) filo.Builtin {
	return func(_ context.Context, args []filo.Value) (filo.Value, error) {
		return tagged(tag, args...), nil
	}
}

// tagOf splits a tagged list into its tag and the remaining items.
func tagOf(v filo.Value) (string, []filo.Value, bool) {
	if v.Kind == filo.KList && len(v.List) > 0 && v.List[0].Kind == filo.KString {
		return v.List[0].Str, v.List[1:], true
	}
	return "", nil, false
}

// numbers converts a run of values to floats, naming the offending position.
func numbers(vs []filo.Value) ([]float64, error) {
	out := make([]float64, len(vs))
	for i, v := range vs {
		n, err := v.AsNumber()
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", i+1, err)
		}
		out[i] = n
	}
	return out, nil
}

// nums is numbers with an exact-arity check, for the many fixed-shape forms.
func nums(tag string, vs []filo.Value, want int, form string) ([]float64, error) {
	if len(vs) != want {
		return nil, fmt.Errorf("%s: want %s", tag, form)
	}
	return numbers(vs)
}

// str reads a positional string argument.
func str(tag string, v filo.Value, what string) (string, error) {
	if v.Kind != filo.KString {
		return "", fmt.Errorf("%s: %s must be a string", tag, what)
	}
	return v.Str, nil
}

// one reads a form that carries exactly one number, e.g. (glow 1.2).
func one(tag string, items []filo.Value) (float64, error) {
	got, err := nums(tag, items, 1, "("+tag+" n)")
	if err != nil {
		return 0, err
	}
	return got[0], nil
}

// oneStr reads a form that carries exactly one string, e.g. (stroke "#80ffff").
func oneStr(tag string, items []filo.Value) (string, error) {
	if len(items) != 1 {
		return "", fmt.Errorf("%s: want (%s \"text\")", tag, tag)
	}
	return str(tag, items[0], "value")
}

// strings reads a run of string arguments, e.g. (tags "ship" "flyable").
func strs(tag string, items []filo.Value) ([]string, error) {
	out := make([]string, len(items))
	for i, v := range items {
		s, err := str(tag, v, fmt.Sprintf("item %d", i+1))
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// num formats a float as the shortest string that parses back to the same value, so a
// round trip through the file never drifts a coordinate.
func num(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// builder is a strings.Builder with a printf shorthand. Emitting a document is a few
// hundred small formatted fragments, and a strings.Builder write cannot fail.
type builder struct{ *strings.Builder }

func (b builder) printf(format string, a ...any) {
	fmt.Fprintf(b.Builder, format, a...)
}

// newEngine registers a document's builtins on a fresh interpreter. RegisterBuiltin
// refuses a name Filo's core already owns, so a clash surfaces here rather than as a
// baffling parse.
func newEngine(sets ...map[string]filo.Builtin) (*filo.Filo, error) {
	f := filo.New()
	for _, set := range sets {
		for name, fn := range set {
			err := f.RegisterBuiltin(name, fn)
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("filoio: register %q: %w", name, err)
			}
		}
	}
	return f, nil
}
