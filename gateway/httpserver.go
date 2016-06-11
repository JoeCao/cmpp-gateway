package gateway

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"math"
)

var pageSize = 5
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
	var page int
	if parameter == "" {
		page = 1
	} else {
		page, _ = strconv.Atoi(parameter)
	}
	count := SCache.LengthOfMoList()
	lastPage, nextPage, totalPage, startRow, endRow := calPages(page, count)
	t, err := template.New("list_message.html").ParseFiles("list_message.html")
	if err != nil {
		fmt.Fprintf(w, "template error %v", err)
		return
	}
	v := SCache.GetSubmits(startRow, endRow)
	ret := map[string]interface{}{
		"data":     v,
		"lastpage": lastPage,
		"nextpage": nextPage,
		"page":     page,
		"totalpage": totalPage,
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
	var page int
	if parameter == "" {
		page = 1
	} else {
		page, _ = strconv.Atoi(parameter)
	}
	count := SCache.LengthOfMoList()
	lastPage, nextPage, totalPage, startRow, endRow := calPages(page, count)
	t, err := template.New("list_mo.html").ParseFiles("list_mo.html")
	if err != nil {
		fmt.Fprintf(w, "template error %v", err)
		return
	}
	v := SCache.GetMoList(startRow, endRow)
	ret := map[string]interface{}{
		"data":     v,
		"lastpage": lastPage,
		"nextpage": nextPage,
		"page":     page,
		"totalpage": totalPage,
	}

	err = t.Execute(w, ret)
	if err != nil {
		fmt.Fprintf(w, "error %v", err)
		return
	}
}

func calPages(page int, length int) (int, int, int, int, int) {
	d := float64(length) / float64(pageSize)
	totalPage := int(math.Ceil(d))
	var lastPage, nextPage, startRow, endRow int
	if page == 1 {
		lastPage = 1
	} else {
		lastPage = page - 1
	}
	startRow = page * pageSize - pageSize
	if page >= totalPage {
		nextPage = totalPage
	} else {
		nextPage = page + 1
	}
	endRow = page * pageSize - 1
	return lastPage, nextPage, totalPage, startRow, endRow
}

func Serve(config *Config) {
	http.HandleFunc("/send", handler)
	http.HandleFunc("/", index)
	http.HandleFunc("/list_message", listMessage)
	http.HandleFunc("/list_mo", listMo)
	log.Fatal(http.ListenAndServe(config.HttpHost + ":" + config.HttpPort, nil))
}
