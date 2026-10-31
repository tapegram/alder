package theme

import (
	"math"
	"strconv"
	"strings"
)

// RGBA is a colour in gamma-encoded sRGB, each channel clipped to 0–1.
type RGBA struct{ R, G, B, A float64 }

// ParseColour parses a CSS Color 4 colour: hex, a named colour, `transparent`, or one of
// rgb/rgba/hsl/hsla/hwb/lab/lch/oklab/oklch/color(). ok is false for anything else
// (`currentColor`, `color-mix(…)`, `light-dark(…)`, `var(…)`, malformed text).
func ParseColour(text string) (RGBA, bool) {
	s := strings.TrimSpace(text)
	if s == "" {
		return RGBA{}, false
	}
	if s[0] == '#' {
		return parseHex(s[1:])
	}
	open := strings.IndexByte(s, '(')
	if open < 0 {
		low := strings.ToLower(s)
		if low == "transparent" {
			return RGBA{}, true
		}
		if h, ok := namedColours[low]; ok {
			return RGBA{float64(h>>16&255) / 255, float64(h>>8&255) / 255, float64(h&255) / 255, 1}, true
		}
		return RGBA{}, false
	}
	if !strings.HasSuffix(s, ")") {
		return RGBA{}, false
	}
	fn := strings.ToLower(strings.TrimSpace(s[:open]))
	args, alpha, ok := splitArgs(s[open+1 : len(s)-1])
	if !ok {
		return RGBA{}, false
	}
	a := 1.0
	if alpha != "" {
		v, ok := parseNumber(alpha, 1)
		if !ok {
			return RGBA{}, false
		}
		a = clamp01(v)
	}
	var r, g, b float64
	if fn == "color" {
		r, g, b, ok = colourFunction(args)
	} else {
		if len(args) != 3 {
			return RGBA{}, false
		}
		r, g, b, ok = colourSpace(fn, args)
	}
	if !ok {
		return RGBA{}, false
	}
	return RGBA{clamp01(r), clamp01(g), clamp01(b), a}, true
}

func parseHex(h string) (RGBA, bool) {
	for _, c := range h {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return RGBA{}, false
		}
	}
	switch len(h) {
	case 3, 4:
		var e []byte
		for i := 0; i < len(h); i++ {
			e = append(e, h[i], h[i])
		}
		h = string(e)
	case 6, 8:
	default:
		return RGBA{}, false
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(h[i:i+2], 16, 8)
		return float64(v) / 255
	}
	c := RGBA{ch(0), ch(2), ch(4), 1}
	if len(h) == 8 {
		c.A = ch(6)
	}
	return c, true
}

// splitArgs splits a function body into its channel arguments and the alpha text after `/`
// (or, for the legacy comma syntax, the fourth argument).
func splitArgs(body string) (args []string, alpha string, ok bool) {
	if strings.ContainsAny(body, "()") {
		return nil, "", false
	}
	main := body
	if i := strings.IndexByte(body, '/'); i >= 0 {
		main, alpha = body[:i], strings.TrimSpace(body[i+1:])
		if alpha == "" || strings.ContainsAny(alpha, "/, ") {
			return nil, "", false
		}
	}
	if strings.Contains(main, ",") {
		if alpha != "" {
			return nil, "", false
		}
		for _, p := range strings.Split(main, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				return nil, "", false
			}
			args = append(args, p)
		}
		if len(args) == 4 {
			args, alpha = args[:3], args[3]
		}
		return args, alpha, true
	}
	return strings.Fields(main), alpha, true
}

// parseNumber reads a number, or a percentage scaled to ref (100% = ref).
func parseNumber(s string, ref float64) (float64, bool) {
	if s == "none" {
		return 0, true
	}
	if p, ok := strings.CutSuffix(s, "%"); ok {
		v, err := strconv.ParseFloat(p, 64)
		return v / 100 * ref, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// parseHue reads an angle in degrees: a bare number, or one with deg, grad, rad or turn.
func parseHue(s string) (float64, bool) {
	if s == "none" {
		return 0, true
	}
	for _, u := range []struct {
		suffix string
		scale  float64
	}{{"deg", 1}, {"grad", 0.9}, {"rad", 180 / math.Pi}, {"turn", 360}} {
		if n, ok := strings.CutSuffix(s, u.suffix); ok {
			v, ok := parseNumber(n, 1)
			return v * u.scale, ok && !strings.HasSuffix(n, "%")
		}
	}
	if strings.HasSuffix(s, "%") {
		return 0, false
	}
	return parseNumber(s, 1)
}

func colourSpace(fn string, a []string) (r, g, b float64, ok bool) {
	num := func(i int, ref float64) float64 {
		v, good := parseNumber(a[i], ref)
		ok = ok && good
		return v
	}
	hue := func(i int) float64 {
		v, good := parseHue(a[i])
		ok = ok && good
		return v
	}
	ok = true
	switch fn {
	case "rgb", "rgba":
		r, g, b = num(0, 255)/255, num(1, 255)/255, num(2, 255)/255
	case "hsl", "hsla":
		h, s, l := hue(0), num(1, 1), num(2, 1)
		r, g, b = hslToRGB(h, s, l)
	case "hwb":
		h, w, bl := hue(0), num(1, 1), num(2, 1)
		if w+bl >= 1 {
			v := w / (w + bl)
			return v, v, v, ok
		}
		r, g, b = hslToRGB(h, 1, 0.5)
		r, g, b = r*(1-w-bl)+w, g*(1-w-bl)+w, b*(1-w-bl)+w
	case "oklab":
		r, g, b = oklabToSRGB(num(0, 1), num(1, 0.4), num(2, 0.4))
	case "oklch":
		l, c, h := num(0, 1), num(1, 0.4), hue(2)
		r, g, b = oklabToSRGB(l, c*cosDeg(h), c*sinDeg(h))
	case "lab":
		r, g, b = labToSRGB(num(0, 100), num(1, 125), num(2, 125))
	case "lch":
		l, c, h := num(0, 100), num(1, 150), hue(2)
		r, g, b = labToSRGB(l, c*cosDeg(h), c*sinDeg(h))
	default:
		return 0, 0, 0, false
	}
	return r, g, b, ok
}

func colourFunction(a []string) (r, g, b float64, ok bool) {
	if len(a) != 4 {
		return 0, 0, 0, false
	}
	var v [3]float64
	for i := range v {
		x, good := parseNumber(a[i+1], 1)
		if !good {
			return 0, 0, 0, false
		}
		v[i] = x
	}
	switch strings.ToLower(a[0]) {
	case "srgb":
		return v[0], v[1], v[2], true
	case "srgb-linear":
		return encode(v[0]), encode(v[1]), encode(v[2]), true
	case "display-p3":
		lr, lg, lb := decode(v[0]), decode(v[1]), decode(v[2])
		x, y, z := mul(p3ToXYZ, lr, lg, lb)
		r, g, b = xyzToSRGB(x, y, z)
		return r, g, b, true
	}
	return 0, 0, 0, false
}

func cosDeg(d float64) float64 { return math.Cos(d * math.Pi / 180) }
func sinDeg(d float64) float64 { return math.Sin(d * math.Pi / 180) }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func hslToRGB(h, s, l float64) (r, g, b float64) {
	h = math.Mod(math.Mod(h, 360)+360, 360)
	s = clamp01(s)
	l = clamp01(l)
	f := func(n float64) float64 {
		k := math.Mod(n+h/30, 12)
		return l - s*math.Min(l, 1-l)*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	return f(0), f(8), f(4)
}

// sRGB transfer function.
func encode(l float64) float64 {
	if a := math.Abs(l); a > 0.0031308 {
		return math.Copysign(1.055*math.Pow(a, 1/2.4)-0.055, l)
	}
	return 12.92 * l
}

func decode(v float64) float64 {
	if a := math.Abs(v); a <= 0.04045 {
		return v / 12.92
	}
	return math.Copysign(math.Pow((math.Abs(v)+0.055)/1.055, 2.4), v)
}

type matrix [3][3]float64

func mul(m matrix, a, b, c float64) (float64, float64, float64) {
	return m[0][0]*a + m[0][1]*b + m[0][2]*c,
		m[1][0]*a + m[1][1]*b + m[1][2]*c,
		m[2][0]*a + m[2][1]*b + m[2][2]*c
}

// Matrices published in CSS Color 4 (sample code, conversions.js).
var (
	xyzD65ToLinSRGB = matrix{
		{12831.0 / 3959, -329.0 / 214, -1974.0 / 3959},
		{-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810},
		{705.0 / 12673, -2585.0 / 12673, 705.0 / 667},
	}
	p3ToXYZ = matrix{
		{608311.0 / 1250200, 189793.0 / 714400, 198249.0 / 1000160},
		{35783.0 / 156275, 247089.0 / 357200, 198249.0 / 2500400},
		{0, 32229.0 / 714400, 5220557.0 / 5000800},
	}
	xyzD50ToD65 = matrix{
		{0.955473421488075, -0.02309845494876471, 0.06325924320057072},
		{-0.0283697093338637, 1.0099953980813041, 0.021041441191917323},
		{0.012314014864481998, -0.020507649298898964, 1.330365926242124},
	}
	oklabToLMSg = matrix{
		{1, 0.3963377773761749, 0.2158037573099136},
		{1, -0.1055613458156586, -0.0638541728258133},
		{1, -0.0894841775298119, -1.2914855480194092},
	}
	lmsToXYZD65 = matrix{
		{1.2268798758459243, -0.5578149944602171, 0.2813910456659647},
		{-0.0405757452148008, 1.1122868032803170, -0.0717110580655164},
		{-0.0763729366746601, -0.4214933324022432, 1.5869240198367816},
	}
)

func xyzToSRGB(x, y, z float64) (r, g, b float64) {
	lr, lg, lb := mul(xyzD65ToLinSRGB, x, y, z)
	return encode(lr), encode(lg), encode(lb)
}

func oklabToSRGB(l, a, b float64) (float64, float64, float64) {
	lp, mp, sp := mul(oklabToLMSg, l, a, b)
	x, y, z := mul(lmsToXYZD65, lp*lp*lp, mp*mp*mp, sp*sp*sp)
	return xyzToSRGB(x, y, z)
}

func labToSRGB(l, a, b float64) (float64, float64, float64) {
	const eps, kappa = 216.0 / 24389, 24389.0 / 27
	fy := (l + 16) / 116
	fx, fz := a/500+fy, fy-b/200
	cube := func(f float64) float64 {
		if f*f*f > eps {
			return f * f * f
		}
		return (116*f - 16) / kappa
	}
	yr := cube(fy)
	if l <= kappa*eps {
		yr = l / kappa
	}
	// D50 white point.
	x, y, z := cube(fx)*0.3457/0.3585, yr, cube(fz)*(1-0.3457-0.3585)/0.3585
	x, y, z = mul(xyzD50ToD65, x, y, z)
	return xyzToSRGB(x, y, z)
}

func init() {
	namedColours = make(map[string]uint32, 148)
	for _, f := range strings.Fields(namedColourList) {
		name, hex, _ := strings.Cut(f, ":")
		v, _ := strconv.ParseUint(hex, 16, 32)
		namedColours[name] = uint32(v)
	}
}

var namedColours map[string]uint32

const namedColourList = `aliceblue:f0f8ff antiquewhite:faebd7 aqua:00ffff aquamarine:7fffd4 azure:f0ffff beige:f5f5dc bisque:ffe4c4
black:000000 blanchedalmond:ffebcd blue:0000ff blueviolet:8a2be2 brown:a52a2a burlywood:deb887 cadetblue:5f9ea0
chartreuse:7fff00 chocolate:d2691e coral:ff7f50 cornflowerblue:6495ed cornsilk:fff8dc crimson:dc143c cyan:00ffff
darkblue:00008b darkcyan:008b8b darkgoldenrod:b8860b darkgray:a9a9a9 darkgreen:006400 darkgrey:a9a9a9
darkkhaki:bdb76b darkmagenta:8b008b darkolivegreen:556b2f darkorange:ff8c00 darkorchid:9932cc darkred:8b0000
darksalmon:e9967a darkseagreen:8fbc8f darkslateblue:483d8b darkslategray:2f4f4f darkslategrey:2f4f4f
darkturquoise:00ced1 darkviolet:9400d3 deeppink:ff1493 deepskyblue:00bfff dimgray:696969 dimgrey:696969
dodgerblue:1e90ff firebrick:b22222 floralwhite:fffaf0 forestgreen:228b22 fuchsia:ff00ff gainsboro:dcdcdc
ghostwhite:f8f8ff gold:ffd700 goldenrod:daa520 gray:808080 green:008000 greenyellow:adff2f grey:808080
honeydew:f0fff0 hotpink:ff69b4 indianred:cd5c5c indigo:4b0082 ivory:fffff0 khaki:f0e68c lavender:e6e6fa
lavenderblush:fff0f5 lawngreen:7cfc00 lemonchiffon:fffacd lightblue:add8e6 lightcoral:f08080 lightcyan:e0ffff
lightgoldenrodyellow:fafad2 lightgray:d3d3d3 lightgreen:90ee90 lightgrey:d3d3d3 lightpink:ffb6c1
lightsalmon:ffa07a lightseagreen:20b2aa lightskyblue:87cefa lightslategray:778899 lightslategrey:778899
lightsteelblue:b0c4de lightyellow:ffffe0 lime:00ff00 limegreen:32cd32 linen:faf0e6 magenta:ff00ff
maroon:800000 mediumaquamarine:66cdaa mediumblue:0000cd mediumorchid:ba55d3 mediumpurple:9370db
mediumseagreen:3cb371 mediumslateblue:7b68ee mediumspringgreen:00fa9a mediumturquoise:48d1cc
mediumvioletred:c71585 midnightblue:191970 mintcream:f5fffa mistyrose:ffe4e1 moccasin:ffe4b5
navajowhite:ffdead navy:000080 oldlace:fdf5e6 olive:808000 olivedrab:6b8e23 orange:ffa500 orangered:ff4500
orchid:da70d6 palegoldenrod:eee8aa palegreen:98fb98 paleturquoise:afeeee palevioletred:db7093
papayawhip:ffefd5 peachpuff:ffdab9 peru:cd853f pink:ffc0cb plum:dda0dd powderblue:b0e0e6 purple:800080
rebeccapurple:663399 red:ff0000 rosybrown:bc8f8f royalblue:4169e1 saddlebrown:8b4513 salmon:fa8072
sandybrown:f4a460 seagreen:2e8b57 seashell:fff5ee sienna:a0522d silver:c0c0c0 skyblue:87ceeb
slateblue:6a5acd slategray:708090 slategrey:708090 snow:fffafa springgreen:00ff7f steelblue:4682b4
tan:d2b48c teal:008080 thistle:d8bfd8 tomato:ff6347 turquoise:40e0d0 violet:ee82ee wheat:f5deb3
white:ffffff whitesmoke:f5f5f5 yellow:ffff00 yellowgreen:9acd32`
