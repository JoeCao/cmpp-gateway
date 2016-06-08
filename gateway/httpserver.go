package gateway

import (
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"html/template"
	"log"
	"net/http"
)

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
	mes := SmsMes{Src: src, Content: content, Dest: dest}
	Messages <- mes
	result, _ := json.Marshal(
		map[string]interface{}{"error": "", "result": 0})
	fmt.Fprintf(w, string(result))
}

func index(w http.ResponseWriter, r *http.Request) {
	findTemplate(w, r, "index.html")

}

func findTemplate(w http.ResponseWriter, r *http.Request, tpl string) {
	t, error := template.New(tpl).ParseFiles(tpl)
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
func listMessage(w http.ResponseWriter, r *http.Request) {
	t, error := template.New("list_message.html").ParseFiles("list_message.html")
	if error != nil {
		fmt.Fprintf(w, "template error %v", error)
		return
	}
	values, err := redis.Strings(RedisConn.Do("LRANGE", "submitlist", 0, -1))
	if err != nil {
		fmt.Println(err)
		return
	}
	v := make([]SmsMes, 0, len(values))
	for _, s := range values {
		mes := SmsMes{}
		json.Unmarshal([]byte(s), &mes)
		v = append(v, mes)
	}

	err = t.Execute(w, v)
	if err != nil {
		fmt.Fprintf(w, "error %v", err)
		return
	}
}

func listMo(w http.ResponseWriter, r *http.Request) {
	t, error := template.New("list_mo.html").ParseFiles("list_mo.html")
	if error != nil {
		fmt.Fprintf(w, "template error %v", error)
		return
	}
	values, err := redis.Strings(RedisConn.Do("LRANGE", "molist", 0, -1))
	if err != nil {
		fmt.Println(err)
		return
	}
	v := make([]SmsMes, 0, len(values))
	for _, s := range values {
		mes := SmsMes{}
		json.Unmarshal([]byte(s), &mes)
		v = append(v, mes)
	}

	err = t.Execute(w, v)
	if err != nil {
		fmt.Fprintf(w, "error %v", err)
		return
	}
}

func Serve(config *Config) {
	http.HandleFunc("/send", handler)
	http.HandleFunc("/", index)
	http.HandleFunc("/list_message", listMessage)
	http.HandleFunc("/list_mo", listMo)
	log.Fatal(http.ListenAndServe(config.HttpHost+":"+config.HttpPort, nil))
}
