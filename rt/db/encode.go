package db

import (
	"fmt"
	"time"

	"alder/rt"
)

// The Encode* helpers turn Alder values into the values a store keeps (see
// the storage mapping in the package doc); the Decode* helpers take what a
// store returns (string, int64, float64, []byte or nil) back, returning an
// error — never panicking — when the stored value has the wrong type.

// EncodeTime is t in UTC as RFC 3339 with nanoseconds ("…Z").
func EncodeTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// DecodeTime parses an EncodeTime value; the result is in UTC.
func DecodeTime(v any) (time.Time, error) {
	s, err := DecodeText(v)
	if err != nil {
		return time.Time{}, fmt.Errorf("db: Time: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("db: Time: %w", err)
	}
	return t.UTC(), nil
}

// EncodeDate is d as "YYYY-MM-DD".
func EncodeDate(d rt.Date) string { return fmt.Sprintf("%04d-%02d-%02d", d.Y, d.M, d.D) }

// DecodeDate parses an EncodeDate value.
func DecodeDate(v any) (rt.Date, error) {
	s, err := DecodeText(v)
	if err != nil {
		return rt.Date{}, fmt.Errorf("db: Date: %w", err)
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return rt.Date{}, fmt.Errorf("db: Date: %w", err)
	}
	return rt.DateOf(t), nil
}

// EncodeBool is 1 for true, 0 for false.
func EncodeBool(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// DecodeBool accepts the integers 0 and 1.
func DecodeBool(v any) (bool, error) {
	n, ok := v.(int64)
	if !ok || (n != 0 && n != 1) {
		return false, fmt.Errorf("db: Bool: want 0 or 1, got %#v", v)
	}
	return n == 1, nil
}

// DecodeText accepts TEXT (string or []byte). Use it for Text, brands and
// enum constructor names.
func DecodeText(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	}
	return "", fmt.Errorf("db: Text: want text, got %#v", v)
}

// DecodeInt accepts INTEGER.
func DecodeInt(v any) (int64, error) {
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("db: Int: want integer, got %#v", v)
	}
	return n, nil
}

// DecodeFloat accepts REAL (and INTEGER, which SQLite may hand back for a
// whole number).
func DecodeFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int64:
		return float64(x), nil
	}
	return 0, fmt.Errorf("db: Float: want real, got %#v", v)
}

// EncodeMaybe is nil for Nothing and enc(x) for Just x.
func EncodeMaybe[T any](m rt.Maybe[T], enc func(T) any) any {
	if !m.Ok {
		return nil
	}
	return enc(m.Val)
}

// DecodeMaybe is Nothing for NULL and Just (dec v) otherwise.
func DecodeMaybe[T any](v any, dec func(any) (T, error)) (rt.Maybe[T], error) {
	if v == nil {
		return rt.Nothing[T](), nil
	}
	x, err := dec(v)
	if err != nil {
		return rt.Nothing[T](), err
	}
	return rt.Just(x), nil
}
