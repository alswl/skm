package pagination

import "testing"

func TestPageCount(t *testing.T) {
	cases := []struct {
		total, size, want int
	}{
		{0, 10, 0},
		{5, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{25, 10, 3},
		{10, 0, 0},
	}
	for _, c := range cases {
		if got := PageCount(c.total, c.size); got != c.want {
			t.Errorf("PageCount(%d,%d) = %d, want %d", c.total, c.size, got, c.want)
		}
	}
}

func TestPage(t *testing.T) {
	cases := []struct {
		total, size, page     int
		wantOffset, wantCount int
	}{
		{25, 10, 0, 0, 10},
		{25, 10, 1, 10, 10},
		{25, 10, 2, 20, 5},
		{25, 10, 9, 20, 5}, // page clamped to last
		{0, 10, 0, 0, 0},
		{25, 10, -3, 0, 10}, // negative clamped to first
	}
	for _, c := range cases {
		got := Page(c.total, c.size, c.page)
		if got.Offset != c.wantOffset || got.Count != c.wantCount {
			t.Errorf("Page(%d,%d,%d) = offset %d count %d, want %d/%d",
				c.total, c.size, c.page, got.Offset, got.Count, c.wantOffset, c.wantCount)
		}
	}
}

func TestClampCursor(t *testing.T) {
	cases := []struct{ cursor, total, want int }{
		{0, 0, 0},
		{-1, 10, 0},
		{3, 10, 3},
		{9, 10, 9},
		{15, 10, 9},
	}
	for _, c := range cases {
		if got := ClampCursor(c.cursor, c.total); got != c.want {
			t.Errorf("ClampCursor(%d,%d) = %d, want %d", c.cursor, c.total, got, c.want)
		}
	}
}
