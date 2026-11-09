package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- CSV/批量导入 ----------

type importRow struct {
	Date     string `json:"date"`
	Type     string `json:"type"`
	Amount   string `json:"amount"`
	Category string `json:"category"`
	Account  string `json:"account"`
	Note     string `json:"note"`
}

// normalizeDate 支持 2026-11-09 / 2026/11/9 / 2026.11.09 / 20261109
func normalizeDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("/", "-", ".", "-").Replace(s)
	for _, layout := range []string{"2006-01-02", "20060102", "2006-1-2", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", strconv.ErrSyntax
}

// parseImportType 支持 支出/收入/expense/income/1/2
func parseImportType(s string) (int, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "1", "支出", "expense", "out":
		return 1, nil
	case "2", "收入", "income", "in":
		return 2, nil
	}
	return 0, strconv.ErrSyntax
}

// parseImportAmount 金额（元），支持 "¥25.60"、"1,234.50"
func parseImportAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("¥", "", "￥", "", ",", "", "，", "", " ", "").Replace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, strconv.ErrSyntax
	}
	return int64(mathRound(f * 100)), nil
}

func mathRound(f float64) float64 {
	if f < 0 {
		return -float64(int64(-f + 0.5))
	}
	return float64(int64(f + 0.5))
}

func (h *hub) handleImport(w http.ResponseWriter, r *http.Request, uid int64) {
	var req struct {
		Rows []importRow `json:"rows"`
	}
	if err := decodeBody(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	if len(req.Rows) == 0 {
		errJSON(w, http.StatusBadRequest, "没有可导入的数据")
		return
	}
	if len(req.Rows) > 1000 {
		errJSON(w, http.StatusBadRequest, "单次最多导入 1000 行")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback()

	imported, skipped := 0, 0
	var failed []map[string]string
	addErr := func(line int, msg string) {
		failed = append(failed, map[string]string{"line": strconv.Itoa(line), "error": msg})
	}

	for i, row := range req.Rows {
		line := i + 1

		date, err := normalizeDate(row.Date)
		if err != nil {
			addErr(line, "日期无效: "+row.Date)
			continue
		}
		typ, err := parseImportType(row.Type)
		if err != nil {
			addErr(line, "类型无效（应为 支出/收入）: "+row.Type)
			continue
		}
		amount, err := parseImportAmount(row.Amount)
		if err != nil {
			addErr(line, "金额无效: "+row.Amount)
			continue
		}
		catName := strings.TrimSpace(row.Category)
		if catName == "" || len(catName) > 20 {
			addErr(line, "分类名为空或过长")
			continue
		}
		accName := strings.TrimSpace(row.Account)
		if accName == "" || len(accName) > 20 {
			addErr(line, "账户名为空或过长")
			continue
		}
		note := strings.TrimSpace(row.Note)
		if len([]rune(note)) > 100 {
			note = string([]rune(note)[:100])
		}

		// 分类：存在则用，不存在自动创建
		var catID int64
		err = tx.QueryRow("SELECT id FROM categories WHERE user_id=? AND name=? AND type=?", uid, catName, typ).Scan(&catID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec("INSERT INTO categories(user_id, name, type) VALUES(?,?,?)", uid, catName, typ)
			if err != nil {
				addErr(line, "创建分类失败: "+catName)
				continue
			}
			catID, _ = res.LastInsertId()
		} else if err != nil {
			addErr(line, "查询分类失败")
			continue
		}

		// 账户：存在则用，不存在自动创建
		var accID int64
		err = tx.QueryRow("SELECT id FROM accounts WHERE user_id=? AND name=?", uid, accName).Scan(&accID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec("INSERT INTO accounts(user_id, name) VALUES(?,?)", uid, accName)
			if err != nil {
				addErr(line, "创建账户失败: "+accName)
				continue
			}
			accID, _ = res.LastInsertId()
		} else if err != nil {
			addErr(line, "查询账户失败")
			continue
		}

		// 幂等：同 日期+类型+金额+分类+账户+备注 已存在则跳过
		var n int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM tx WHERE user_id=? AND type=? AND amount=? AND category_id=? AND account_id=? AND tx_date=? AND note=?`,
			uid, typ, amount, catID, accID, date, note).Scan(&n)
		if n > 0 {
			skipped++
			continue
		}

		if _, err := tx.Exec("INSERT INTO tx(user_id, type, amount, category_id, account_id, tx_date, note) VALUES(?,?,?,?,?,?,?)",
			uid, typ, amount, catID, accID, date, note); err != nil {
			addErr(line, "写入失败")
			continue
		}
		imported++
	}

	// 全有或全无：任何一行有问题，整批回滚并返回行级错误
	if len(failed) > 0 {
		tx.Rollback()
		writeJSON(w, http.StatusOK, map[string]any{"imported": 0, "skipped": 0, "failed": len(failed), "errors": failed})
		return
	}
	if err := tx.Commit(); err != nil {
		errJSON(w, http.StatusInternalServerError, "提交失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": 0})
}
