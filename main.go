package main

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed templates/* static/*
var contentFS embed.FS

type Course struct {
	ID                int      `json:"id"`
	Slug              string   `json:"slug"`
	Title             string   `json:"title"`
	Tagline           string   `json:"tagline"`
	Category          string   `json:"category"`
	CategoryName      string   `json:"category_name"`
	Level             string   `json:"level"`
	Duration          string   `json:"duration"`
	Format            string   `json:"format"`
	Price             int      `json:"price"`
	DiscountPrice     int      `json:"discount_price"`
	SpotsLeft         int      `json:"spots_left"`
	Badge             string   `json:"badge"`
	StartDate         string   `json:"start_date"`
	Highlights        []string `json:"highlights"`
	Syllabus          []Module `json:"syllabus"`
	FormattedPrice    string   `json:"-"`
	FormattedDiscount string   `json:"-"`
}

type Module struct {
	Day   string `json:"day"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type Lead struct {
	ID          int    `json:"id"`
	OrderID     string `json:"order_id"`
	CourseID    int    `json:"course_id"`
	CourseSlug  string `json:"course_slug"`
	Name        string `json:"name"`
	WhatsApp    string `json:"whatsapp"`
	Email       string `json:"email"`
	Notes       string `json:"notes"`
	Amount      int    `json:"amount"`
	UniqueCode  int    `json:"unique_code"`
	TotalAmount int    `json:"total_amount"`
	Status      string `json:"status"`
	ProofFile   string `json:"proof_file"`
	CreatedAt   string `json:"created_at"`
}

type PageData struct {
	Courses   []Course
	NextBatch string
}

type InvoicePageData struct {
	Order          Lead
	Course         Course
	FormattedTotal string
	WhatsAppURL    string
}

var (
	db        *sql.DB
	tpl       *template.Template
	uploadDir string
)

func formatRupiah(amount int) string {
	s := fmt.Sprintf("%d", amount)
	n := len(s)
	if n <= 3 {
		return "Rp " + s
	}
	var res []string
	rem := n % 3
	if rem > 0 {
		res = append(res, s[:rem])
	}
	for i := rem; i < n; i += 3 {
		res = append(res, s[i:i+3])
	}
	return "Rp " + strings.Join(res, ".")
}

func generateOrderID() string {
	b := make([]byte, 2)
	rand.Read(b)
	now := time.Now()
	return fmt.Sprintf("KOG-%02d%02d-%s", now.Year()%100, now.Month(), strings.ToUpper(hex.EncodeToString(b)))
}

func initDB() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "courses.db"
	}

	// Ensure parent directory exists
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		os.MkdirAll(dir, 0755)
	}

	connStr := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)
	var err error
	db, err = sql.Open("sqlite", connStr)
	if err != nil {
		log.Fatalf("Gagal membuka database SQLite: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS courses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT UNIQUE NOT NULL,
		title TEXT NOT NULL,
		tagline TEXT NOT NULL,
		category TEXT NOT NULL,
		category_name TEXT NOT NULL,
		level TEXT NOT NULL,
		duration TEXT NOT NULL,
		format TEXT NOT NULL,
		price INTEGER NOT NULL,
		discount_price INTEGER NOT NULL,
		spots_left INTEGER NOT NULL,
		badge TEXT,
		start_date TEXT NOT NULL,
		highlights TEXT NOT NULL,
		syllabus TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS leads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_id TEXT UNIQUE NOT NULL,
		course_id INTEGER,
		course_slug TEXT,
		name TEXT NOT NULL,
		whatsapp TEXT NOT NULL,
		email TEXT,
		notes TEXT,
		amount INTEGER NOT NULL DEFAULT 0,
		unique_code INTEGER NOT NULL DEFAULT 0,
		total_amount INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		proof_file TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatalf("Gagal inisialisasi tabel: %v", err)
	}

	seedOrUpdateCourses()
}

func seedOrUpdateCourses() {
	courses := []Course{
		{
			Slug:          "jago-hermes",
			Title:         "Jago Hermes: Belajar Agentic AI dari Nol",
			Tagline:       "Kuasai framework agentic AI Hermes dari Nous Research: tool calling, custom skills, persistent memory, dan integrasi terminal mandiri.",
			Category:      "hermes",
			CategoryName:  "Hermes Agent",
			Level:         "Semua Level (Pemula - Menengah)",
			Duration:      "4 Pertemuan • 12 Jam Hands-On",
			Format:        "Live Coding + Free VPS Linux 1 Bulan",
			Price:         600000,
			DiscountPrice: 300000,
			SpotsLeft:     15,
			Badge:         "🎁 Free VPS 1 Bulan",
			StartDate:     "10 - 13 Okt 2026",
			Highlights: []string{
				"Free Akses Cloud VPS Linux Selama 1 Bulan Penuh",
				"Instalasi & Konfigurasi Hermes Agent di Server Mandiri",
				"Pembuatan Modular Skills, Memory Engine & Safety Rails",
				"Koneksi ke Model Open-Weight (Ollama/vLLM) & AI Gateway",
				"Autonomous Coding & Bot Automation Project",
			},
			Syllabus: []Module{
				{Day: "Hari 1", Title: "Fondasi Agentic AI & Setup Cloud VPS", Desc: "Memahami konsep otonomi agent vs chatbot biasa, setup VPS Linux pribadi (termasuk free 1 bulan), dan instalasi environment Hermes."},
				{Day: "Hari 2", Title: "Skills System & Persistent Memory Engine", Desc: "Membangun custom skills modular, konfigurasi sistem memori jangka panjang (MEMORY.md/USER.md), dan tata kelola context window."},
				{Day: "Hari 3", Title: "Tool Calling, Subagents & Local LLM Routing", Desc: "Menghubungkan Hermes ke model open-source lokal via Ollama/vLLM dan gateway 9Router, orkestrasi subagents untuk tugas paralel."},
				{Day: "Hari 4", Title: "Autonomous Real-World Project & Deployment", Desc: "Membangun proyek otomasi nyata: agent pengolah dokumen, bot monitoring server, dan integrasi background daemon di Linux."},
			},
		},
		{
			Slug:          "autonomous-ai-agents",
			Title:         "Autonomous AI Agents & Tool Calling",
			Tagline:       "Rancang agent cerdas mandiri dengan eksekusi kode, tool use, dan multi-agent orchestration.",
			Category:      "agents",
			CategoryName:  "AI Agents",
			Level:         "Intermediate",
			Duration:      "4 Pertemuan • 12 Jam Hands-On",
			Format:        "Live Coding + Private Repository",
			Price:         1399000,
			DiscountPrice: 699000,
			SpotsLeft:     4,
			Badge:         "🔥 Paling Diminati",
			StartDate:     "14 - 17 Okt 2026",
			Highlights: []string{
				"Arsitektur ReAct, Plan-and-Solve & Custom Loop",
				"Integrasi Model Context Protocol (MCP) Client & Server",
				"Sandboxed Code Execution & Tool Permission Gates",
				"Multi-Agent Tree Coordination (Parent-Child Routing)",
			},
			Syllabus: []Module{
				{Day: "Hari 1", Title: "Fondasi Agentic Loop & Tool Schemas", Desc: "Membangun loop penalar mandiri, native function calling schema, parsing output JSON deterministik, dan memory scratchpad."},
				{Day: "Hari 2", Title: "Model Context Protocol (MCP) Integration", Desc: "Menghubungkan agent ke dunia luar via standar MCP: terminal shell, filesystem, database, dan web browsing tools."},
				{Day: "Hari 3", Title: "Multi-Agent System & Task Delegation", Desc: "Pemisahan peran: Planner Agent, Worker Agent, dan Reviewer/Auditor Agent. Mekanisme passing context dan synthesis."},
				{Day: "Hari 4", Title: "Production Hardening & Safety Rails", Desc: "Watchdog loop breaker, mitigasi halusinasi pemanggilan fungsi, rate-limiting, dan deployment ke Linux system service."},
			},
		},
		{
			Slug:          "selfhosted-llms",
			Title:         "Self-Hosted LLMs & Local Inference Engineering",
			Tagline:       "Jalankan model AI open-source (Llama, DeepSeek, Qwen) di server lokal tanpa bayar API cloud.",
			Category:      "local-llm",
			CategoryName:  "Local LLMs",
			Level:         "All Levels",
			Duration:      "3 Pertemuan • 9 Jam Hands-On",
			Format:        "Live Lab Server + Ready-to-use Docker Stack",
			Price:         990000,
			DiscountPrice: 499000,
			SpotsLeft:     6,
			Badge:         "⚡ Hemat Biaya API",
			StartDate:     "19 - 21 Okt 2026",
			Highlights: []string{
				"High-Throughput vLLM & Ollama Engine Setup",
				"Quantization (GGUF, AWQ, EXL2) & VRAM Sizing",
				"AI Gateway Router (9Router / LiteLLM) dengan Fallback",
				"Sizing Hardware: GPU Server vs CPU-Only Inference",
			},
			Syllabus: []Module{
				{Day: "Hari 1", Title: "Arsitektur Model Open-Weight & Engine Serving", Desc: "Memilih arsitektur model open-source, setup vLLM dengan PagedAttention, dan streaming OpenAI-compatible REST API."},
				{Day: "Hari 2", Title: "Quantization & Memory Optimization", Desc: "Teknik kompresi model (GGUF vs AWQ), perbandingan degradasi kualitas, dan benchmarking token-per-second pada RAM/GPU."},
				{Day: "Hari 3", Title: "Private AI Gateway & Proxy Management", Desc: "Membangun gerbang AI privat dengan load balancing, round-robin fallback ke cloud saat server penuh, dan token metrics."},
			},
		},
		{
			Slug:          "workflow-automation",
			Title:         "AI Workflow Automation & Operations (n8n + DB)",
			Tagline:       "Otomasi proses bisnis dan administrasi nyata: hubungkan AI ke database, WhatsApp, dan bot pesan.",
			Category:      "automation",
			CategoryName:  "Automations",
			Level:         "Beginner-Intermediate",
			Duration:      "4 Pertemuan • 12 Jam Hands-On",
			Format:        "Hands-On Pipeline + Template Flow Siap Pakai",
			Price:         1190000,
			DiscountPrice: 599000,
			SpotsLeft:     5,
			Badge:         "💼 Siap Pakai di Bisnis",
			StartDate:     "26 - 29 Okt 2026",
			Highlights: []string{
				"Instalasi Self-Hosted n8n + AI Agent Nodes",
				"Database Triggers (Supabase pg_cron + AI Webhooks)",
				"WhatsApp Messaging Gateway (WAHA) + Typing Delay",
				"Parsing Dokumen Massal: PDF/Excel ke Database Otomatis",
			},
			Syllabus: []Module{
				{Day: "Hari 1", Title: "Setup Self-Hosted n8n & Node AI Lanjutan", Desc: "Menjalankan n8n tanpa batas eksekusi, konfigurasi LLM Chain, Memory, dan dynamic routing berbasis kondisi data."},
				{Day: "Hari 2", Title: "Otomasi Database: Supabase & pg_cron Triggers", Desc: "Memicu automasi saat data baru masuk di tabel, cron job pelaporan cerdas tanpa server tambahan, dan REST webhook payload."},
				{Day: "Hari 3", Title: "Integrasi WhatsApp & Telegram Gateway", Desc: "Koneksi ke gateway WAHA / Telegram Bot, humanized typing delay, anti-banned delivery queue, dan interaktif command dispatcher."},
				{Day: "Hari 4", Title: "Pipeline Ekstraksi Dokumen ke Database", Desc: "Ekstraksi data PDF Surat Tugas/Invoice ke format JSON terstruktur dan otomatis melakukan upsert ke PostgreSQL."},
			},
		},
		{
			Slug:          "production-rag",
			Title:         "Production RAG & Semantic Vector Search",
			Tagline:       "Bangun sistem pencarian dokumen perusahaan dengan akurasi tinggi dan bebas halusinasi.",
			Category:      "rag",
			CategoryName:  "RAG Systems",
			Level:         "Intermediate",
			Duration:      "3 Pertemuan • 9 Jam Hands-On",
			Format:        "Live Coding + Complete Source Code",
			Price:         1190000,
			DiscountPrice: 589000,
			SpotsLeft:     7,
			Badge:         "🔍 Akurasi Tinggi",
			StartDate:     "02 - 04 Nov 2026",
			Highlights: []string{
				"Chunking Heuristik & Embedding Strategy",
				"PostgreSQL + pgvector (HNSW Indexing)",
				"Hybrid Search (Keyword FTS5 + Dense Cosine Vector)",
				"Cross-Encoder Reranking & Retrieval Evaluation",
			},
			Syllabus: []Module{
				{Day: "Hari 1", Title: "Chunking Lanjutan & Dense Vector Generation", Desc: "Mencegah context leakage, chunking semantik vs fixed-size, dan pemilihan embedding model multilingual."},
				{Day: "Hari 2", Title: "Vector Database: pgvector & HNSW Indexing", Desc: "Setup PostgreSQL pgvector, konfigurasi parameter index HNSW untuk pencarian sub-milidetik, dan filtering metadata."},
				{Day: "Hari 3", Title: "Hybrid Search & Cross-Encoder Reranking", Desc: "Menggabungkan Full-Text Search (BM25/FTS) dengan vector search, pipeline reranker, dan mitigasi halusinasi fakta."},
			},
		},
	}

	stmt, err := db.Prepare(`
		INSERT INTO courses (slug, title, tagline, category, category_name, level, duration, format, price, discount_price, spots_left, badge, start_date, highlights, syllabus)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			title = excluded.title,
			tagline = excluded.tagline,
			category = excluded.category,
			category_name = excluded.category_name,
			level = excluded.level,
			duration = excluded.duration,
			format = excluded.format,
			price = excluded.price,
			discount_price = excluded.discount_price,
			spots_left = excluded.spots_left,
			badge = excluded.badge,
			start_date = excluded.start_date,
			highlights = excluded.highlights,
			syllabus = excluded.syllabus
	`)
	if err != nil {
		log.Fatalf("Prepare insert error: %v", err)
	}
	defer stmt.Close()

	for _, c := range courses {
		hlJSON, _ := json.Marshal(c.Highlights)
		sylJSON, _ := json.Marshal(c.Syllabus)
		stmt.Exec(c.Slug, c.Title, c.Tagline, c.Category, c.CategoryName, c.Level, c.Duration, c.Format, c.Price, c.DiscountPrice, c.SpotsLeft, c.Badge, c.StartDate, string(hlJSON), string(sylJSON))
	}
	log.Println("Database courses synchronized!")
}

func getCourses() ([]Course, error) {
	rows, err := db.Query("SELECT id, slug, title, tagline, category, category_name, level, duration, format, price, discount_price, spots_left, badge, start_date, highlights, syllabus FROM courses ORDER BY CASE WHEN slug = 'jago-hermes' THEN 0 ELSE id END ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Course
	for rows.Next() {
		var c Course
		var hlStr, sylStr string
		err := rows.Scan(&c.ID, &c.Slug, &c.Title, &c.Tagline, &c.Category, &c.CategoryName, &c.Level, &c.Duration, &c.Format, &c.Price, &c.DiscountPrice, &c.SpotsLeft, &c.Badge, &c.StartDate, &hlStr, &sylStr)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(hlStr), &c.Highlights)
		json.Unmarshal([]byte(sylStr), &c.Syllabus)
		c.FormattedPrice = formatRupiah(c.Price)
		c.FormattedDiscount = formatRupiah(c.DiscountPrice)
		list = append(list, c)
	}
	return list, nil
}

func getTelegramCredentials() (string, string) {
	token := os.Getenv("TELEGRAM_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		if content, err := os.ReadFile("/etc/waha-telegram.env"); err == nil {
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "TELEGRAM_TOKEN=") {
					token = strings.Trim(strings.TrimPrefix(line, "TELEGRAM_TOKEN="), "\"' \r\n")
				} else if strings.HasPrefix(line, "TELEGRAM_CHAT_ID=") {
					chatID = strings.Trim(strings.TrimPrefix(line, "TELEGRAM_CHAT_ID="), "\"' \r\n")
				}
			}
		}
	}
	return token, chatID
}

func sendTelegramNewOrderAlert(lead Lead, course Course) {
	token, chatID := getTelegramCredentials()
	if token == "" || chatID == "" {
		return
	}

	text := fmt.Sprintf(
		"🔔 *PENDAFTARAN & INVOICE BARU!*\n\n"+
			"📚 *Kelas:* %s\n"+
			"🏷 *Order ID:* `%s`\n"+
			"👤 *Nama:* %s\n"+
			"📱 *WhatsApp:* `%s`\n"+
			"💵 *Total Tagihan:* *%s*\n"+
			"🏦 *Rekening:* BSI `7150150803` a.n SHAIDUL MAULA\n"+
			"📝 *Catatan:* %s\n"+
			"⏰ *Waktu:* %s WIB\n\n"+
			"Status: Menunggu transfer dari peserta.",
		course.Title, lead.OrderID, lead.Name, lead.WhatsApp,
		formatRupiah(lead.TotalAmount), lead.Notes, time.Now().Format("02 Jan 2006, 15:04"),
	)

	payload, _ := json.Marshal(map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	go func() {
		apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
		resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(payload))
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()
}

func sendTelegramProofPhoto(lead Lead, course Course, filePath string) error {
	token, chatID := getTelegramCredentials()
	if token == "" || chatID == "" {
		return fmt.Errorf("missing telegram credentials")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("chat_id", chatID)

	caption := fmt.Sprintf(
		"💰 *BUKTI TRANSFER PEMBAYARAN BARU!*\n\n"+
			"📚 *Kelas:* %s\n"+
			"🏷 *Order ID:* `%s`\n"+
			"👤 *Nama:* %s\n"+
			"📱 *WhatsApp:* `%s`\n"+
			"💵 *Nominal:* *%s*\n"+
			"🏦 *Tujuan:* BSI `7150150803` (a.n SHAIDUL MAULA)\n"+
			"⏰ *Waktu Upload:* %s WIB\n\n"+
			"Mohon periksa mutasi BSI Mobile Anda.",
		course.Title, lead.OrderID, lead.Name, lead.WhatsApp,
		formatRupiah(lead.TotalAmount), time.Now().Format("02 Jan 2006, 15:04"),
	)
	writer.WriteField("caption", caption)
	writer.WriteField("parse_mode", "Markdown")

	part, err := writer.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return err
	}
	io.Copy(part, file)
	writer.Close()

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", token)
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendPhoto error: HTTP %d %s", resp.StatusCode, string(respBytes))
	}

	return nil
}

func main() {
	uploadDir = os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	os.MkdirAll(uploadDir, 0755)

	initDB()

	funcMap := template.FuncMap{
		"formatRupiah": formatRupiah,
		"toJSON": func(v interface{}) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"urlQuery": url.QueryEscape,
	}
	tpl = template.Must(template.New("").Funcs(funcMap).ParseFS(contentFS, "templates/*.html"))

	http.Handle("/static/", http.FileServer(http.FS(contentFS)))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		courses, err := getCourses()
		if err != nil {
			http.Error(w, "Gagal memuat katalog kelas", http.StatusInternalServerError)
			return
		}

		data := PageData{
			Courses:   courses,
			NextBatch: "10 Oktober 2026",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("Template render error: %v", err)
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/invoice/", func(w http.ResponseWriter, r *http.Request) {
		orderID := strings.TrimPrefix(r.URL.Path, "/invoice/")
		orderID = strings.TrimSpace(orderID)
		if orderID == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		var lead Lead
		err := db.QueryRow(
			"SELECT id, order_id, course_id, course_slug, name, whatsapp, email, notes, amount, unique_code, total_amount, status, COALESCE(proof_file, ''), created_at FROM leads WHERE order_id = ?",
			orderID,
		).Scan(&lead.ID, &lead.OrderID, &lead.CourseID, &lead.CourseSlug, &lead.Name, &lead.WhatsApp, &lead.Email, &lead.Notes, &lead.Amount, &lead.UniqueCode, &lead.TotalAmount, &lead.Status, &lead.ProofFile, &lead.CreatedAt)

		if err != nil {
			http.Error(w, "Invoice tidak ditemukan atau telah kedaluwarsa", http.StatusNotFound)
			return
		}

		var course Course
		var hlStr, sylStr string
		db.QueryRow(
			"SELECT id, slug, title, tagline, category, category_name, level, duration, format, price, discount_price, spots_left, badge, start_date, highlights, syllabus FROM courses WHERE slug = ?",
			lead.CourseSlug,
		).Scan(&course.ID, &course.Slug, &course.Title, &course.Tagline, &course.Category, &course.CategoryName, &course.Level, &course.Duration, &course.Format, &course.Price, &course.DiscountPrice, &course.SpotsLeft, &course.Badge, &course.StartDate, &hlStr, &sylStr)

		waMsg := fmt.Sprintf(
			"Halo Admin Lab AI, saya sudah melakukan pembayaran kelas:\n\n"+
				"*Order ID:* %s\n"+
				"*Kelas:* %s\n"+
				"*Nama:* %s\n"+
				"*Nominal Transfer:* %s\n"+
				"*Tujuan:* BSI 7150150803 a.n SHAIDUL MAULA\n\n"+
				"Bukti transfer terlampir. Mohon diverifikasi. Terima kasih!",
			lead.OrderID, course.Title, lead.Name, formatRupiah(lead.TotalAmount),
		)
		waURL := fmt.Sprintf("https://wa.me/6282246846944?text=%s", url.QueryEscape(waMsg))

		invoiceData := InvoicePageData{
			Order:          lead,
			Course:         course,
			FormattedTotal: formatRupiah(lead.TotalAmount),
			WhatsAppURL:    waURL,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tpl.ExecuteTemplate(w, "invoice.html", invoiceData); err != nil {
			log.Printf("Invoice template error: %v", err)
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var lead Lead
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&lead); err != nil {
				http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
				return
			}
		} else {
			r.ParseForm()
			lead.Name = r.FormValue("name")
			lead.WhatsApp = r.FormValue("whatsapp")
			lead.Email = r.FormValue("email")
			lead.CourseSlug = r.FormValue("course_slug")
			lead.Notes = r.FormValue("notes")
		}

		lead.Name = strings.TrimSpace(lead.Name)
		lead.WhatsApp = strings.TrimSpace(lead.WhatsApp)
		if lead.Name == "" || lead.WhatsApp == "" {
			http.Error(w, "Nama dan WhatsApp wajib diisi", http.StatusBadRequest)
			return
		}

		var course Course
		err := db.QueryRow("SELECT id, slug, title, discount_price FROM courses WHERE slug = ?", lead.CourseSlug).
			Scan(&course.ID, &course.Slug, &course.Title, &course.DiscountPrice)
		if err != nil {
			db.QueryRow("SELECT id, slug, title, discount_price FROM courses LIMIT 1").
				Scan(&course.ID, &course.Slug, &course.Title, &course.DiscountPrice)
		}

		lead.CourseID = course.ID
		lead.CourseSlug = course.Slug
		lead.OrderID = generateOrderID()
		lead.Amount = course.DiscountPrice
		lead.UniqueCode = 0
		lead.TotalAmount = course.DiscountPrice
		lead.Status = "pending"

		_, err = db.Exec(
			"INSERT INTO leads (order_id, course_id, course_slug, name, whatsapp, email, notes, amount, unique_code, total_amount, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			lead.OrderID, lead.CourseID, lead.CourseSlug, lead.Name, lead.WhatsApp, lead.Email, lead.Notes, lead.Amount, lead.UniqueCode, lead.TotalAmount, lead.Status,
		)
		if err != nil {
			log.Printf("Error saving lead: %v", err)
			http.Error(w, "Gagal membuat invoice", http.StatusInternalServerError)
			return
		}

		sendTelegramNewOrderAlert(lead, course)

		invoiceURL := "/invoice/" + lead.OrderID
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":         true,
			"order_id":        lead.OrderID,
			"total_amount":    lead.TotalAmount,
			"formatted_total": formatRupiah(lead.TotalAmount),
			"redirect_url":    invoiceURL,
			"wa_url":          invoiceURL,
		})
	})

	http.HandleFunc("/api/upload-proof", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		r.ParseMultipartForm(10 << 20)

		orderID := strings.TrimSpace(r.FormValue("order_id"))
		if orderID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Order ID tidak valid"})
			return
		}

		file, header, err := r.FormFile("proof")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Berkas bukti tidak ditemukan"})
			return
		}
		defer file.Close()

		var lead Lead
		err = db.QueryRow(
			"SELECT id, order_id, course_id, course_slug, name, whatsapp, total_amount, unique_code FROM leads WHERE order_id = ?",
			orderID,
		).Scan(&lead.ID, &lead.OrderID, &lead.CourseID, &lead.CourseSlug, &lead.Name, &lead.WhatsApp, &lead.TotalAmount, &lead.UniqueCode)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invoice tidak ditemukan"})
			return
		}

		var course Course
		db.QueryRow("SELECT title FROM courses WHERE slug = ?", lead.CourseSlug).Scan(&course.Title)

		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		filename := fmt.Sprintf("%s_%d%s", lead.OrderID, time.Now().Unix(), ext)
		savePath := filepath.Join(uploadDir, filename)

		out, err := os.Create(savePath)
		if err != nil {
			log.Printf("Gagal menyimpan berkas bukti: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Gagal menyimpan berkas di server"})
			return
		}
		defer out.Close()
		io.Copy(out, file)

		db.Exec("UPDATE leads SET status = 'proof_uploaded', proof_file = ? WHERE order_id = ?", filename, lead.OrderID)

		go func() {
			if err := sendTelegramProofPhoto(lead, course, savePath); err != nil {
				log.Printf("Error sending photo to Telegram: %v", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Bukti berhasil diunggah",
		})
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": "kelas-ai",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	log.Printf("🚀 Kelas AI Web Server running on port :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
