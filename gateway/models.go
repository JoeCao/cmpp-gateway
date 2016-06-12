package gateway

import "time"

type SmsMes struct {
	Src             string
	Dest            string
	Content         string
	MsgId           string
	Created         time.Time
	SubmitResult    uint32
	DelivleryResult uint32
}

type MesSlice []SmsMes

func (c MesSlice) Len() int {
	return len(c)
}

func (c MesSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c MesSlice) Less(i, j int) bool {
	return c[i].Created.Before(c[j].Created)
}
