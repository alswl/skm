package pagination

// PageInfo describes a visible window over a list.
type PageInfo struct {
	Total    int // total items
	PageSize int // items per page
	Page     int // 0-based page index
	Offset   int // index of the first item on this page
	Count    int // items on this page
}

// PageCount returns the number of pages for total items at pageSize.
func PageCount(total, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	if total <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}

// ClampPage clamps a 0-based page into [0, PageCount-1].
func ClampPage(page, total, pageSize int) int {
	if page < 0 {
		return 0
	}
	last := PageCount(total, pageSize) - 1
	if page > last {
		return last
	}
	return page
}

// Page computes the window for a 0-based page.
func Page(total, pageSize, page int) PageInfo {
	p := ClampPage(page, total, pageSize)
	offset := 0
	count := 0
	if p >= 0 {
		offset = p * pageSize
		if offset > total {
			offset = total
		}
		count = pageSize
		if offset+count > total {
			count = total - offset
		}
		if count < 0 {
			count = 0
		}
	}
	return PageInfo{
		Total:    total,
		PageSize: pageSize,
		Page:     p,
		Offset:   offset,
		Count:    count,
	}
}

// ClampCursor clamps a cursor (selected index) into [0, total-1], returning 0
// for an empty list.
func ClampCursor(cursor, total int) int {
	if total <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= total {
		return total - 1
	}
	return cursor
}
