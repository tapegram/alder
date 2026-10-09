package std

import "alder/rt"

func ResultMap[E, A, B any](f func(A) B, r rt.Result[E, A]) rt.Result[E, B] {
	if !r.IsOk {
		return rt.Err[E, B](r.Err)
	}
	return rt.Ok[E](f(r.Val))
}

func ResultMapErr[E, F, A any](f func(E) F, r rt.Result[E, A]) rt.Result[F, A] {
	if r.IsOk {
		return rt.Ok[F](r.Val)
	}
	return rt.Err[F, A](f(r.Err))
}

func ResultAndThen[E, A, B any](f func(A) rt.Result[E, B], r rt.Result[E, A]) rt.Result[E, B] {
	if !r.IsOk {
		return rt.Err[E, B](r.Err)
	}
	return f(r.Val)
}

func ResultWithDefault[E, A any](d A, r rt.Result[E, A]) A {
	if r.IsOk {
		return r.Val
	}
	return d
}

func ResultToMaybe[E, A any](r rt.Result[E, A]) rt.Maybe[A] {
	if r.IsOk {
		return rt.Just(r.Val)
	}
	return rt.Nothing[A]()
}
