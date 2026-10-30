package std

import "testing"

func TestBytes_Ops(t *testing.T) {
	b := []byte{1, 2, 3, 4}
	eq(t, "empty", BytesEmpty(), []byte{})
	eq(t, "isEmpty", BytesIsEmpty(BytesEmpty()), true)
	eq(t, "slice", BytesSlice(1, 3, b), []byte{2, 3})
	eq(t, "slice negative", BytesSlice(-2, 4, b), []byte{3, 4})
	eq(t, "slice clamps", BytesSlice(-9, 99, b), []byte{1, 2, 3, 4})
	eq(t, "slice empty when start >= end", BytesSlice(3, 1, b), []byte{})
	eq(t, "append", BytesAppend([]byte{1}, []byte{2}), []byte{1, 2})
	eq(t, "concat", BytesConcat([][]byte{{1}, {}, {2, 3}}), []byte{1, 2, 3})
	eq(t, "concat empty", BytesConcat(nil), []byte{})
	eq(t, "toList", BytesToList([]byte{0, 255}), []int64{0, 255})
	eq(t, "toList empty", BytesToList([]byte{}), []int64{})
	eq(t, "toList nil", BytesToList(nil), []int64{})
	eq(t, "fromList empty", BytesFromList([]int64{}), []byte{})
	eq(t, "fromList nil", BytesFromList(nil), []byte{})
	eq(t, "slice of empty", BytesSlice(0, 5, []byte{}), []byte{})
	eq(t, "slice of nil", BytesSlice(-1, 1, nil), []byte{})
	eq(t, "fromList masks", BytesFromList([]int64{256, -1, 65}), []byte{0, 255, 65})
}

func TestBytes_NoAliasing(t *testing.T) {
	a := make([]byte, 1, 8)
	a[0] = 1
	x := BytesAppend(a, []byte{2})
	y := BytesAppend(a, []byte{3})
	eq(t, "append x", x, []byte{1, 2})
	eq(t, "append y", y, []byte{1, 3})
	b := []byte{1, 2, 3, 4}
	BytesSlice(0, 2, b)[0] = 9
	eq(t, "slice does not alias", b[0], byte(1))
}

func TestBytes_TextSliceRules(t *testing.T) {
	eq(t, "positive", TextSlice(1, 3, "héllo"), "él")
	eq(t, "negative", TextSlice(-2, 5, "héllo"), "lo")
	eq(t, "clamps", TextSlice(-9, 99, "héllo"), "héllo")
	eq(t, "start >= end", TextSlice(3, 1, "héllo"), "")
}
