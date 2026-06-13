package httptransport

import (
	"math"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 12, 9, 30, 15, 123456789, time.UTC)
	enc := encodeCursor(at, 77)
	gotT, gotID, err := decodeCursor(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !gotT.Equal(at) || gotID != 77 {
		t.Fatalf("got (%v, %d), want (%v, 77)", gotT, gotID, at)
	}
}

func TestEmptyCursorIsFirstPageSentinel(t *testing.T) {
	gotT, gotID, err := decodeCursor("")
	if err != nil {
		t.Fatal(err)
	}
	if !gotT.After(time.Now().AddDate(100, 0, 0)) || gotID != math.MaxInt64 {
		t.Fatalf("first-page sentinel must sort after every real row, got (%v, %d)", gotT, gotID)
	}
}

func TestMalformedCursorsRejected(t *testing.T) {
	for _, c := range []string{"!!!", "bm90LWEtY3Vyc29y", "MjAyNnwxfDI"} {
		if _, _, err := decodeCursor(c); err == nil {
			t.Fatalf("cursor %q must be rejected", c)
		}
	}
}
