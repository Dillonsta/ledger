package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func openDB(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(dataDir, "ledger.db")) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	dbh, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := dbh.Ping(); err != nil {
		return nil, err
	}
	if err := migrate(dbh); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := seed(dbh); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return dbh, nil
}

func migrate(dbh *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS accounts(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(user_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS categories(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			type INTEGER NOT NULL CHECK(type IN (1,2)),
			UNIQUE(user_id, name, type)
		)`,
		`CREATE TABLE IF NOT EXISTS tx(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type INTEGER NOT NULL CHECK(type IN (1,2)),
			amount INTEGER NOT NULL CHECK(amount > 0),
			category_id INTEGER NOT NULL REFERENCES categories(id),
			account_id INTEGER NOT NULL REFERENCES accounts(id),
			tx_date TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tx_user_date ON tx(user_id, tx_date)`,
		`CREATE TABLE IF NOT EXISTS budgets(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
			month TEXT NOT NULL,
			amount INTEGER NOT NULL CHECK(amount >= 0),
			UNIQUE(user_id, category_id, month)
		)`,
	}
	for _, s := range stmts {
		if _, err := dbh.Exec(s); err != nil {
			return fmt.Errorf("%q: %w", s[:32], err)
		}
	}
	return nil
}

var (
	defaultExpenseCategories = []string{"餐饮", "交通", "购物", "居住", "娱乐", "医疗", "教育", "通讯", "其他"}
	defaultIncomeCategories  = []string{"工资", "奖金", "理财", "兼职", "其他"}
	defaultAccounts          = []string{"现金", "微信", "支付宝", "银行卡"}
)

// seed 首次启动：创建初始用户并预置默认分类/账户。
// 后续多人使用时扩展注册接口即可，所有表都已按 user_id 隔离。
func seed(dbh *sql.DB) error {
	var n int
	if err := dbh.QueryRow("SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	user := envOr("LEDGER_ADMIN_USER", "admin")
	pass := envOr("LEDGER_ADMIN_PASS", "admin123")
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := dbh.Exec("INSERT INTO users(username, password_hash) VALUES(?,?)", user, string(hash))
	if err != nil {
		return err
	}
	uid, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, name := range defaultExpenseCategories {
		if _, err := dbh.Exec("INSERT INTO categories(user_id, name, type) VALUES(?,?,1)", uid, name); err != nil {
			return err
		}
	}
	for _, name := range defaultIncomeCategories {
		if _, err := dbh.Exec("INSERT INTO categories(user_id, name, type) VALUES(?,?,2)", uid, name); err != nil {
			return err
		}
	}
	for _, name := range defaultAccounts {
		if _, err := dbh.Exec("INSERT INTO accounts(user_id, name) VALUES(?,?)", uid, name); err != nil {
			return err
		}
	}
	log.Printf("已创建初始用户 %q（密码取自 LEDGER_ADMIN_PASS），并预置默认分类与账户", user)
	return nil
}
