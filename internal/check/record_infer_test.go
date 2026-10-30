package check

import (
	"strings"
	"testing"
)

// An unsigned top-level function whose result is a literal mentioning its parameter is
// generalised as on main: the literal is structural for good, so each use instantiates it afresh
// and a named record expected of it is a mismatch with the hint, never a shared decision.
func TestRecordLitInference_InferModuleGeneric(t *testing.T) {
	src := `module M

type Box a = { v: a }

unbox : Box a -> a
unbox b = b.v

mk x = { v: x }

one = (mk 1).v + 1

word = (mk "s").v ++ "t"

boxed = unbox (mk 1)
`
	_, bag := checkWith(t, map[string]string{"M.alder": src}, Options{InferModule: "M"})
	got := render(bag)
	if strings.Count(got, "E0003") != 1 || !strings.Contains(got, "E0003 expected Box _, got { v: Int }; the record has Box _'s fields: declare it as Box _ where it is made") {
		t.Fatalf("want one E0003 with the hint for `boxed`, got:\n%s", got)
	}
}

// A generalised literal's field keeps its own instance per use, as on main.
func TestRecordLitInference_InferModuleFieldPerUse(t *testing.T) {
	src := `module M

it = { l: [] }

ints = it.l == [1]

texts = it.l == ["a"]
`
	_, bag := checkWith(t, map[string]string{"M.alder": src}, Options{InferModule: "M"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("want no errors, got:\n%s", got)
	}
}

// A record literal typed with nothing expected takes the named record type it first meets (#3).

const curveDecls = `type Curve = { median: Float, sigma: Float, perDoubling: Float }

type Summary = { t0: Int, label: Text }

type Row = { name: Text, value: Float }

type Shown = { name: Text, shown: Bool }

reportTime : Curve -> Float -> Float
reportTime c t = c.median + c.sigma * t + c.perDoubling
`

func curveModule(rest string) string { return "module M\n\n" + curveDecls + "\n" + rest }

func errorTexts(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, d := range checkModule(t, src) {
		out = append(out, d.Code+" "+d.Message)
	}
	return out
}

func TestRecordLitInference_Shapes(t *testing.T) {
	cases := map[string]string{
		"eq": `ok : Result Text Summary -> Bool
ok r = r == Ok { t0: 1, label: "a" }
`,
		"eq-literal-left": `ok : Summary -> Bool
ok s = { label: "a", t0: 1 } == s
`,
		"pipeline": `shown : List Row -> List Shown
shown rows = rows |> List.map (\r -> { name: r.name, shown: r.value > 0.0 })
`,
		"chain": `shown : List Row -> List Shown
shown rows =
  rows
    |> List.map (\r -> { name: r.name, shown: r.value > 0.0 })
    |> List.filter (\s -> s.shown)
`,
		"let": `clamped : Float -> Float
clamped t = {
  let wide = { median: 1.0, sigma: 2.0, perDoubling: 0.5 }
  reportTime wide t
}
`,
		"ctor-let": `guess : Bool -> Maybe Curve
guess known = {
  let m = Just { median: 1.0, sigma: 0.1, perDoubling: 0.0 }
  if known then m else Nothing
}
`,
		"update": `clamped : Float -> Float
clamped t = {
  let wide = { median: 1.0, sigma: 2.0, perDoubling: 0.5 }
  let narrow = { wide | sigma: 1.0 }
  reportTime narrow t
}
`,
		"list-of-literals": `curves : List Curve
curves = {
  let xs = [{ median: 1.0, sigma: 2.0, perDoubling: 0.5 }, { median: 2.0, sigma: 2.0, perDoubling: 0.5 }]
  xs
}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if got := errorTexts(t, curveModule(body)); len(got) != 0 {
				t.Fatalf("want no errors, got %q", got)
			}
		})
	}
}

// An unsigned top-level value (eval/REPL) is generalised; its literal stays undecided and its
// first use decides it.
func TestRecordLitInference_InferModule(t *testing.T) {
	src := curveModule(`wide = { median: 1.0, sigma: 2.0, perDoubling: 0.5 }

clamped : Float -> Float
clamped t = reportTime wide t
`)
	_, bag := checkWith(t, map[string]string{"M.alder": src}, Options{InferModule: "M"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("want no errors, got %s", got)
	}
}

// Two named types with the same fields: the literal becomes the first one it meets, and the
// other is a plain nominal mismatch with no hint.
func TestRecordLitInference_PointVec(t *testing.T) {
	src := `module M

type Point = { x: Float, y: Float }

type Vec = { x: Float, y: Float }

atPoint : Point -> Float
atPoint p = p.x

along : Vec -> Float
along v = v.y

both : Float
both = {
  let p = { x: 1.0, y: 2.0 }
  atPoint p + along p
}
`
	got := errorTexts(t, src)
	if len(got) != 1 || got[0] != "E0003 expected Vec, got Point" {
		t.Fatalf("got %q, want one `expected Vec, got Point` with no hint", got)
	}
}
