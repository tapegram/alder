package std

import "testing"

func TestText_Pad(t *testing.T) {
	eq(t, "padLeft", TextPadLeft(2, "0", "7"), "07")
	eq(t, "padRight", TextPadRight(5, ".", "ab"), "ab...")
	eq(t, "padLeft long enough", TextPadLeft(2, "0", "123"), "123")
	eq(t, "padLeft empty pad", TextPadLeft(5, "", "a"), "a")
	eq(t, "padLeft multi pad cut at outer edge", TextPadLeft(5, "ab", "x"), "ababx")
	eq(t, "padRight multi pad", TextPadRight(5, "ab", "x"), "xabab")
	eq(t, "padLeft multi pad odd cut", TextPadLeft(6, "ab", "x"), "bababx")
	eq(t, "padRight multi pad odd cut", TextPadRight(6, "ab", "x"), "xababa")
	eq(t, "padLeft code points", TextPadLeft(3, "é", "a"), "ééa")
}

func TestText_LeftRight(t *testing.T) {
	eq(t, "left", TextLeft(2, "héllo"), "hé")
	eq(t, "left negative", TextLeft(-1, "abc"), "")
	eq(t, "left past end", TextLeft(9, "abc"), "abc")
	eq(t, "right", TextRight(2, "abc"), "bc")
	eq(t, "right negative", TextRight(-1, "abc"), "")
	eq(t, "right past start", TextRight(9, "abc"), "abc")
	eq(t, "right non-ASCII", TextRight(2, "héllo€"), "o€")
	eq(t, "dropLeft negative", TextDropLeft(-1, "abc"), "abc")
	eq(t, "dropLeft non-ASCII", TextDropLeft(2, "héllo€"), "llo€")
	eq(t, "dropLeft", TextDropLeft(1, "abc"), "bc")
	eq(t, "dropRight", TextDropRight(1, "abc"), "ab")
	eq(t, "dropRight past end", TextDropRight(9, "abc"), "")
}

func TestText_RepeatReverseTrim(t *testing.T) {
	eq(t, "repeat", TextRepeat(3, "ab"), "ababab")
	eq(t, "repeat zero", TextRepeat(0, "ab"), "")
	eq(t, "repeat negative", TextRepeat(-2, "ab"), "")
	eq(t, "reverse", TextReverse("aé😀"), "😀éa")
	eq(t, "trimLeft", TextTrimLeft("  a "), "a ")
	eq(t, "trimRight", TextTrimRight(" a \n"), " a")
	eq(t, "concat", TextConcat([]string{"a", "", "bc"}), "abc")
}

func TestText_LinesWords(t *testing.T) {
	eq(t, "lines", TextLines("a\nb\n"), []string{"a", "b"})
	eq(t, "lines crlf", TextLines("a\r\nb"), []string{"a", "b"})
	eq(t, "lines empty", TextLines(""), []string{})
	eq(t, "lines blank middle", TextLines("a\n\nb"), []string{"a", "", "b"})
	eq(t, "words", TextWords("  a  b "), []string{"a", "b"})
	eq(t, "words blank", TextWords("   "), []string{})
}

func TestText_Indexes(t *testing.T) {
	eq(t, "indexes overlap", TextIndexes("aa", "aaa"), []int64{0, 1})
	eq(t, "indexes code points", TextIndexes("é", "éaé"), []int64{0, 2})
	eq(t, "indexes empty needle", TextIndexes("", "abc"), []int64{})
	eq(t, "indexes none", TextIndexes("z", "abc"), []int64{})
}

func TestText_CodePoints(t *testing.T) {
	eq(t, "toCodePoints", TextToCodePoints("A😀"), []int64{65, 0x1F600})
	eq(t, "fromCodePoints", TextFromCodePoints([]int64{104, 105}), "hi")
	eq(t, "fromCodePoints invalid", TextFromCodePoints([]int64{-1, 0xD800, 0x110000}), "���")
}
