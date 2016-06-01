package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"html/template"
)

type SmsSlice []SmsMessage

func (c SmsSlice) Len() int {
	return len(c)
}

func (c SmsSlice) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c SmsSlice) Less(i, j int) bool {
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
	mes := SmsMessage{Src: src, Content: content, Dest: dest}
	Messages <- mes
	result, _ := json.Marshal(
		map[string]interface{}{"error": "", "result": 0})
	fmt.Fprintf(w, string(result))
}

func handlerMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	m := SubmitCache.Items()
	v := make(SmsSlice, 0, len(m))
	for _, value := range m {
		//强转value为SmsMessage
		v = append(v, value.(SmsMessage))
	}
	result, _ := json.Marshal(v)
	fmt.Fprintf(w, string(result))
}
type Person struct {
    UserName string
}


func index(w http.ResponseWriter, r *http.Request) {
	t, error := template.New("index.html").ParseFiles("index.html")
	if error != nil {
		fmt.Fprintf(w, "template error %v", error)
		return
	}

	err := t.Execute(w, struct{}{})
	if err != nil {
		fmt.Fprintf(w, "error %v", err)
		return
	}

}

func Serve(config *Config) {
	http.HandleFunc("/send", handler)
	http.HandleFunc("/messages", handlerMessage)
	http.HandleFunc("/", index)
	log.Fatal(http.ListenAndServe(config.HttpHost+":"+config.HttpPort, nil))
}
