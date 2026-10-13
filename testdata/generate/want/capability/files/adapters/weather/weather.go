// Package weather holds the Go adapter of capability Weather.Weather, written by
// `alder new capability`: each method returns an error until you write it (the Alder book,
// chapter 7, "Implementing a capability").
package weather

import (
	"context"
	"errors"

	"alder/rt"
	"app"
)

// Weather is the adapter of Weather.Weather.
func Weather() app.Cap__Weather__Weather { return adapterWeather{} }

// adapterWeather implements Weather.Weather.
type adapterWeather struct{}

// Current is Weather.current: not implemented yet.
func (adapterWeather) Current(ctx context.Context, args app.Args__Weather__Weather__Current) (rt.Maybe[float64], error) {
	var zero rt.Maybe[float64]
	return zero, errors.New("not implemented")
}
