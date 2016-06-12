package pages

import "math"

type Page struct {
	CurrentPage int
	LastPage    int
	NextPage    int
	TotalPage   int
	StartRow    int
	EndRow      int
	IsEnd       bool
	IsFirst     bool
	PageSize    int
}

func NewPage(current, size, length int) Page {
	page := Page{CurrentPage: current, PageSize: size}
	page.calPages(length)
	return page
}

func (p *Page) calPages(length int) {
	d := float64(length) / float64(p.PageSize)
	p.TotalPage = int(math.Ceil(d))
	if p.CurrentPage == 1 {
		p.LastPage = 1
		p.IsFirst = true
	} else {
		p.LastPage = p.CurrentPage - 1
		p.IsFirst = false
	}
	p.StartRow = p.CurrentPage*p.PageSize - p.PageSize
	if p.CurrentPage >= p.TotalPage {
		p.NextPage = p.TotalPage
		p.IsEnd = true
	} else {
		p.NextPage = p.CurrentPage + 1
		p.IsEnd = false
	}
	p.EndRow = p.CurrentPage*p.PageSize - 1
}
