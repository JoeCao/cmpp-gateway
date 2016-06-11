package gateway

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"github.com/JoeCao/cmpp-gateway/pages"
)

var pageSize = 10
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
	r.ParseForm()
	parameter := r.Form.Get("page")
	var c_page int
	if parameter == "" {
		c_page = 1
	} else {
		c_page, _ = strconv.Atoi(parameter)
	}
	count := SCache.LengthOfMoList()
	page := pages.NewPage(c_page, pageSize, count)
	t, err := template.New("list_message.html").ParseFiles("list_message.html")
	if err != nil {
		fmt.Fprintf(w, "template error %v", err)
		return
	}
	v := SCache.GetSubmits(page.StartRow, page.EndRow)
	ret := map[string]interface{}{
		"data":     v,
		"page":     page,
	}
	err = t.Execute(w, ret)
	if err != nil {
		fmt.Fprintf(w, "error %v", err)
		return
	}
}

func listMo(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	parameter := r.Form.Get("page")
	var c_page int
	if parameter == "" {
		c_page = 1
	} else {
		c_page, _ = strconv.Atoi(parameter)
	}
	count := SCache.LengthOfMoList()
	page := pages.NewPage(c_page, pageSize, count)
	t, err := template.New("list_mo.html").ParseFiles("list_mo.html")
	if err != nil {
		fmt.Fprintf(w, "template error %v", err)
		return
	}
	v := SCache.GetMoList(page.StartRow, page.EndRow)
	ret := map[string]interface{}{
		"data":     v,
		"page":     page,
	}

	err = t.Execute(w, ret)
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
	log.Fatal(http.ListenAndServe(config.HttpHost + ":" + config.HttpPort, nil))
}
