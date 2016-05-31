package main

import (
	"encoding/json"
	"fmt"
	"github.com/JoeCao/cmpp-gateway/gateway"
	"log"
	"net/http"
	//"go/types"
	"sort"
)

func main() {
	http.HandleFunc("/send", handler)
	http.HandleFunc("/messages", handlerMessage)
	go gateway.StartInput()
	log.Fatal(http.ListenAndServe(":8000", nil))

}

type SmsSlice []gateway.SmsMessage

func (c SmsSlice) Len() int{
	return len(c)
}

func (c SmsSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c SmsSlice) Less(i, j int) bool{
	return c[i].Created.After(c[j].Created)
}



// handler echoes the HTTP request.
func handler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Print(err)
	}
	src := r.Form.Get("src")
	content := r.Form.Get("cont")
	dest := r.Form.Get("dest")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if src == "" || content == "" || dest == "" {
		result, _ := json.Marshal(
			map[string]interface{}{"result": -1, "error": "请输入 参数'src' 'dest' 'const' 缺一不可"})
		fmt.Fprintf(w, string(result))
		return
	}
	mes := gateway.SmsMessage{Src: src, Content: content, Dest: dest}
	gateway.Messages <- mes
	result, _ := json.Marshal(
		map[string]interface{}{"error": "", "result": 0})
	fmt.Fprintf(w, string(result))
}

func handlerMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	m := gateway.SubmitCache.Items()
	v := make(SmsSlice, 0, len(m))
	for _, value := range m{
		v = append(v, value.(gateway.SmsMessage))
	}
	news := sort.Reverse(v)
	result, _ := json.Marshal(news)
	fmt.Fprintf(w, string(result))
}

