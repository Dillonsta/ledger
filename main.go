package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed web
var webFS embed.FS

func main() {
	dataDir := envOr("LEDGER_DATA", "/data")
	addr := envOr("LEDGER_ADDR", ":8300")

	dbh, err := openDB(dataDir)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer dbh.Close()

	authn, err := newAuth(dataDir)
	if err != nil {
		log.Fatalf("初始化认证失败: %v", err)
	}

	h := &hub{db: dbh, auth: authn}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/login", h.handleLogin)
	mux.HandleFunc("GET /api/me", h.requireAuth(h.handleMe))
	mux.HandleFunc("POST /api/password", h.requireAuth(h.handleChangePassword))

	mux.HandleFunc("GET /api/tx", h.requireAuth(h.handleListTx))
	mux.HandleFunc("POST /api/tx", h.requireAuth(h.handleCreateTx))
	mux.HandleFunc("PUT /api/tx/{id}", h.requireAuth(h.handleUpdateTx))
	mux.HandleFunc("DELETE /api/tx/{id}", h.requireAuth(h.handleDeleteTx))

	mux.HandleFunc("GET /api/categories", h.requireAuth(h.handleListCategories))
	mux.HandleFunc("POST /api/categories", h.requireAuth(h.handleCreateCategory))
	mux.HandleFunc("DELETE /api/categories/{id}", h.requireAuth(h.handleDeleteCategory))

	mux.HandleFunc("GET /api/accounts", h.requireAuth(h.handleListAccounts))
	mux.HandleFunc("POST /api/accounts", h.requireAuth(h.handleCreateAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", h.requireAuth(h.handleDeleteAccount))

	mux.HandleFunc("GET /api/budgets", h.requireAuth(h.handleGetBudgets))
	mux.HandleFunc("PUT /api/budgets", h.requireAuth(h.handlePutBudget))

	mux.HandleFunc("GET /api/report", h.requireAuth(h.handleReport))

	static, err := fs.Sub(webFS, "web/static")
	if err != nil {
		log.Fatalf("静态资源错误: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(static)))

	srv := &http.Server{
		Addr:              addr,
		Handler:           logMW(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("ledger 已启动: %s (数据目录 %s)", addr, dataDir)
	log.Fatal(srv.ListenAndServe())
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/api/report" { // 报表页轮询较频繁，降噪
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}
