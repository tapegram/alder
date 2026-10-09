package std

import "alder/rt"

func MaybeMap[A, B any](f func(A) B, m rt.Maybe[A]) rt.Maybe[B] {
	if !m.Ok {
		return rt.Nothing[B]()
	}
	return rt.Just(f(m.Val))
}

func MaybeAndThen[A, B any](f func(A) rt.Maybe[B], m rt.Maybe[A]) rt.Maybe[B] {
	if !m.Ok {
		return rt.Nothing[B]()
	}
	return f(m.Val)
}

func MaybeWithDefault[A any](d A, m rt.Maybe[A]) A { return rt.OrElse(m, d) }

func MaybeIsJust[A any](m rt.Maybe[A]) bool { return m.Ok }

func MaybeToList[A any](m rt.Maybe[A]) []A {
	if !m.Ok {
		return []A{}
	}
	return []A{m.Val}
}
