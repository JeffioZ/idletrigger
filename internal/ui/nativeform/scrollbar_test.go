package nativeform

import "testing"

func TestScrollbarThumbGeometry(t *testing.T) {
	tests := []struct {
		name                       string
		total, page, position      int
		trackTop, trackBottom, min int32
		wantTop, wantBottom        int32
	}{
		{name: "no scrolling", total: 10, page: 10, trackTop: 2, trackBottom: 102, min: 22, wantTop: 2, wantBottom: 102},
		{name: "start", total: 100, page: 20, position: 0, trackTop: 0, trackBottom: 100, min: 22, wantTop: 0, wantBottom: 22},
		{name: "middle", total: 100, page: 20, position: 40, trackTop: 0, trackBottom: 100, min: 22, wantTop: 39, wantBottom: 61},
		{name: "end", total: 100, page: 20, position: 80, trackTop: 0, trackBottom: 100, min: 22, wantTop: 78, wantBottom: 100},
		{name: "clamped", total: 100, page: 20, position: 1000, trackTop: 0, trackBottom: 100, min: 22, wantTop: 78, wantBottom: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			top, bottom := scrollbarThumb(test.total, test.page, test.position, test.trackTop, test.trackBottom, test.min)
			if top != test.wantTop || bottom != test.wantBottom {
				t.Fatalf("scrollbarThumb() = (%d, %d), want (%d, %d)", top, bottom, test.wantTop, test.wantBottom)
			}
		})
	}
}
