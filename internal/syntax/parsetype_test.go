package syntax

import "testing"

func TestParseType(t *testing.T) {
	isWild := func(ty Type) bool {
		tv, ok := ty.(*TypeVar)
		return ok && tv.Name == "_"
	}
	tests := []struct {
		name     string
		src      string
		wildcard bool
		check    func(t *testing.T, ty Type)
		wantDiag int // -1: at least one; 0: none; n: exactly n
	}{
		{"fun", "List a -> a -> List a", false, func(t *testing.T, ty Type) {
			if ft, ok := ty.(*FunType); !ok || len(ft.Params) != 2 {
				t.Errorf("got %#v", ty)
			}
		}, 0},
		{"qualified", "Text.Builder", false, func(t *testing.T, ty Type) {
			if tn, ok := ty.(*TypeName); !ok || tn.Module != "Text" || tn.Name != "Builder" {
				t.Errorf("got %#v", ty)
			}
		}, 0},
		{"record", "{ index: Int, item: a }", false, func(t *testing.T, ty Type) {
			if rt, ok := ty.(*RecordType); !ok || len(rt.Fields) != 2 {
				t.Errorf("got %#v", ty)
			}
		}, 0},
		{"wildcard param", "Text -> _ -> List Text", true, func(t *testing.T, ty Type) {
			if ft, ok := ty.(*FunType); !ok || len(ft.Params) != 2 || !isWild(ft.Params[1]) {
				t.Errorf("got %#v", ty)
			}
		}, 0},
		{"wildcard arg", "Maybe _", true, func(t *testing.T, ty Type) {
			if tn, ok := ty.(*TypeName); !ok || len(tn.Args) != 1 || !isWild(tn.Args[0]) {
				t.Errorf("got %#v", ty)
			}
		}, 0},
		{"wildcard off", "Text -> _", false, nil, -1},
		{"dangling arrow", "Text ->", true, nil, -1},
		{"leftover", "Text Int )", true, nil, -1},
		{"empty", "", true, func(t *testing.T, ty Type) {
			if ty != nil {
				t.Errorf("got %#v", ty)
			}
		}, 1},
		{"spaces", "  \n ", true, func(t *testing.T, ty Type) {
			if ty != nil {
				t.Errorf("got %#v", ty)
			}
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ty, diags := ParseType(tc.src, tc.wildcard)
			switch {
			case tc.wantDiag < 0 && len(diags) == 0:
				t.Errorf("want a diagnostic, got none")
			case tc.wantDiag >= 0 && len(diags) != tc.wantDiag:
				t.Errorf("want %d diagnostics, got %v", tc.wantDiag, diags)
			}
			if tc.check != nil {
				tc.check(t, ty)
			}
		})
	}
}

func TestParseFile_UnderscoreTypeStillRejected(t *testing.T) {
	_, diags := ParseFile("m.alder", []byte("module M\n\nf : Text -> _\nf = \\x -> x\n"))
	if len(diags) == 0 {
		t.Fatal("want a diagnostic for `_` in a source file type")
	}
}
