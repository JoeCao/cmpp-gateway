package gateway

import (
	"encoding/json"
	"fmt"
	"github.com/JoeCao/cmpp-gateway/pages"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"path/filepath"
)

var pageSize = 5
var templates *template.Template

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
	// Fallback to old templates if new ones are not available
	if templates == nil {
		findTemplate(w, r, "index.html")
		return
	}

	// Get stats from Redis
	stats := SCache.GetStats()
	totalReceived := SCache.Length("list_mo")

	data := struct {
		ActivePage string
		Stats      map[string]int
		Config     *Config
		DefaultSrc string
	}{
		ActivePage: "home",
		Stats: map[string]int{
			"TotalSubmitted": stats["total"],
			"TotalSuccess":   stats["success"],
			"TotalFailed":    stats["failed"],
			"TotalReceived":  totalReceived,
		},
		Config:     config,
		DefaultSrc: config.SmsAccessNo,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderTemplate(w, "index", data); err != nil {
		log.Printf("Template execution error: %v", err)
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
	}{
		ActivePage: activePage,
		Data:       v,
		Page:       page,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderTemplate(w, listName, data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func listSubmits(w http.ResponseWriter, r *http.Request) {
	listMessage(w, r, "list_message", "list_message")
}

func listMo(w http.ResponseWriter, r *http.Request) {
	listMessage(w, r, "list_mo", "list_mo")
}

// initTemplates initializes all templates with helper functions
func initTemplates() error {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"eq":  func(a, b interface{}) bool { return a == b },
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

	log.Printf("Loaded %d template files", len(allFiles))
	return nil
}

func Serve(cfg *Config) {
	config = cfg

	// Initialize templates
	if err := initTemplates(); err != nil {
		log.Printf("Warning: Failed to load new templates: %v", err)
		log.Printf("Falling back to old template system")
		// Set templates to nil to trigger fallback
		templates = nil
	}

	http.HandleFunc("/send", handler)
	http.HandleFunc("/", index)
	http.HandleFunc("/list_message", listSubmits)
	http.HandleFunc("/list_mo", listMo)

	log.Printf("HTTP server starting on %s:%s", config.HttpHost, config.HttpPort)
	log.Fatal(http.ListenAndServe(config.HttpHost+":"+config.HttpPort, nil))
}
