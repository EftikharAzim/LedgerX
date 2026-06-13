package service

import (
	"testing"
	"time"
)

func TestValidateLegsZeroSum(t *testing.T) {
	cases := []struct {
		name     string
		legs     []Leg
		external bool
		wantErr  error
	}{
		{"balanced pair", []Leg{{1, -500}, {2, 500}}, false, nil},
		{"unbalanced pair", []Leg{{1, -500}, {2, 400}}, false, ErrUnbalanced},
		{"single leg without external", []Leg{{1, 500}}, false, ErrUnbalanced},
		{"single leg with external offset", []Leg{{1, 500}}, true, nil},
		{"zero amount leg", []Leg{{1, 0}, {2, 0}}, false, ErrZeroAmount},
		{"three balanced legs", []Leg{{1, -300}, {2, 100}, {3, 200}}, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateLegs(c.legs, c.external)
			if c.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErr != nil && err != c.wantErr {
				t.Fatalf("got %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestRequestHashDeterministic(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	in := balancedInput{
		UserID:   1,
		Legs:     []Leg{{AccountID: 2, AmountMinor: 500}},
		Currency: "USD",

		OccurredAt: at,
		Note:       "coffee",
	}
	first := requestHash(in)
	second := requestHash(in)
	if first != second {
		t.Fatal("same input must hash identically")
	}

	// Same instant in a different zone must hash the same (UTC-normalized).
	zone := in
	zone.OccurredAt = at.In(time.FixedZone("X", 6*3600))
	if requestHash(in) != requestHash(zone) {
		t.Fatal("equivalent instants in different zones must hash identically")
	}

	changed := in
	changed.Legs = []Leg{{AccountID: 2, AmountMinor: 501}}
	if requestHash(in) == requestHash(changed) {
		t.Fatal("different payloads must hash differently")
	}
}
