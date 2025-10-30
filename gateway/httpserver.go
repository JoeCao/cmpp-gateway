package gateway

import (
	"encoding/json"
	"fmt"
	"html/template"
    "log"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/JoeCao/cmpp-gateway/pages"
)

var pageSize = 5
var templates *template.Template

// handler echoes the HTTP request.
func handler(w http.ResponseWriter, r *http.Request) {
    if err := r.ParseForm(); err != nil {
        Warnf("[HTTP] 解析表单失败: %v", err)
	}
    // 服务未就绪时拒绝发送
    if !IsCmppReady() {
        w.Header().Set("Content-Type", "application/json; charset=UTF-8")
        result, _ := json.Marshal(
            map[string]interface{}{"result": -2, "error": "CMPP 未连接，服务暂不可用"})
        fmt.Fprintf(w, string(result))
        return
    }
	src := r.Form.Get("src")
	content := r.Form.Get("cont")
	dest := r.Form.Get("dest")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	// src 是可选的，只检查 content 和 dest
	if content == "" || dest == "" {
		result, _ := json.Marshal(
			map[string]interface{}{"result": -1, "error": "请输入参数 'dest' 和 'cont'"})
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
	// Fallback to old templates if new ones are not available
	if templates == nil {
		findTemplate(w, r, "index.html")
		return
	}

	// Get stats from Redis
	stats := SCache.GetStats()
	totalReceived := SCache.Length("list_mo")

	// 检查 Redis 是否启用
	isRedisEnabled := config.CacheType == "redis"
	// 或者通过类型断言检查 SCache 是否为 *Cache (Redis 实现)
	if _, ok := SCache.(*Cache); ok {
		isRedisEnabled = true
	}

    data := struct {
		ActivePage      string
		Stats           map[string]int
		Config          *Config
		DefaultSrc      string
		IsRedisEnabled  bool
        ServiceReady    bool
	}{
		ActivePage: "home",
		Stats: map[string]int{
			"TotalSubmitted": stats["total"],
			"TotalSuccess":   stats["success"],
			"TotalFailed":    stats["failed"],
			"TotalReceived":  totalReceived,
		},
		Config:         config,
		DefaultSrc:     config.SmsAccessNo,
        IsRedisEnabled: isRedisEnabled,
        ServiceReady:   IsCmppReady(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
    if err := renderTemplate(w, "index", data); err != nil {
        Errorf("[TPL] 渲染 index 失败: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderTemplate renders a page template with base layout
func renderTemplate(w http.ResponseWriter, name string, data interface{}) error {
	// Execute the base template which will pull in the page-specific content
	return templates.ExecuteTemplate(w, "base.html", data)
}

// findTemplate is the old template system for fallback
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

func listMessage(w http.ResponseWriter, r *http.Request, listName string, activePage string) {
	r.ParseForm()
	parameter := r.Form.Get("page")
	var c_page int
	if parameter == "" {
		c_page = 1
	} else {
		c_page, _ = strconv.Atoi(parameter)
	}
	count := SCache.Length(listName)
	page := pages.NewPage(c_page, pageSize, count)
	v := SCache.GetList(listName, page.StartRow, page.EndRow)

    data := struct {
		ActivePage string
		Data       *[]SmsMes
		Page       pages.Page
        ServiceReady bool
	}{
		ActivePage: activePage,
		Data:       v,
		Page:       page,
        ServiceReady: IsCmppReady(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
    if err := renderTemplate(w, listName, data); err != nil {
        Errorf("[TPL] 渲染 %s 失败: %v", listName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func listSubmits(w http.ResponseWriter, r *http.Request) {
	listMessage(w, r, "list_message", "list_message")
}

func listMo(w http.ResponseWriter, r *http.Request) {
	listMessage(w, r, "list_mo", "list_mo")
}

// getStats 返回实时统计数据的API接口
func getStats(w http.ResponseWriter, r *http.Request) {
	stats := SCache.GetStats()
	totalReceived := SCache.Length("list_mo")

	response := map[string]int{
		"total":    stats["total"],
		"success":  stats["success"],
		"failed":   stats["failed"],
		"received": totalReceived,
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	json.NewEncoder(w).Encode(response)
}

// initTemplates initializes all templates with helper functions
func initTemplates() error {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"eq":  func(a, b interface{}) bool { return a == b },
		"isSuccess": func(result uint32) bool {
			return result == 0
		},
		"isWaiting": func(result uint32) bool {
			return result == 65535
		},
		"pageRange": func(current, total int) []int {
			// Generate page range for pagination (max 5 pages)
			start := current - 2
			if start < 1 {
				start = 1
			}
			end := start + 4
			if end > total {
				end = total
				start = end - 4
				if start < 1 {
					start = 1
				}
			}
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
	}

	var err error
	templates = template.New("").Funcs(funcMap)

	// Parse all template files
	layoutFiles, err := filepath.Glob("templates/layouts/*.html")
	if err != nil {
		return fmt.Errorf("failed to load layout templates: %v", err)
	}

	partialFiles, err := filepath.Glob("templates/partials/*.html")
	if err != nil {
		return fmt.Errorf("failed to load partial templates: %v", err)
	}

	pageFiles, err := filepath.Glob("templates/pages/*.html")
	if err != nil {
		return fmt.Errorf("failed to load page templates: %v", err)
	}

	allFiles := append(layoutFiles, partialFiles...)
	allFiles = append(allFiles, pageFiles...)

	if len(allFiles) == 0 {
		return fmt.Errorf("no template files found")
	}

	templates, err = templates.ParseFiles(allFiles...)
	if err != nil {
		return fmt.Errorf("failed to parse templates: %v", err)
	}

    Infof("[TPL] 加载模板文件 %d 个", len(allFiles))
	return nil
}

func Serve(cfg *Config) {
	config = cfg

	// Initialize templates
    if err := initTemplates(); err != nil {
        Warnf("[TPL] 新模板加载失败: %v，回退旧模板系统", err)
		// Set templates to nil to trigger fallback
		templates = nil
	}

	http.HandleFunc("/send", handler)
	http.HandleFunc("/", index)
	http.HandleFunc("/list_message", listSubmits)
	http.HandleFunc("/list_mo", listMo)
	http.HandleFunc("/api/stats", getStats)

    Infof("[HTTP] 服务启动: %s:%s", config.HttpHost, config.HttpPort)
    log.Fatal(http.ListenAndServe(config.HttpHost+":"+config.HttpPort, nil))
}
