package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type hub struct {
	db   *sql.DB
	auth *authService
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

var (
	dateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
)

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func validMonth(s string) bool {
	if !monthRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01", s)
	return err == nil
}

// ---------- 流水 ----------

type txReq struct {
	Type       int    `json:"type"` // 1=支出 2=收入
	Amount     int64  `json:"amount"` // 单位：分
	CategoryID int64  `json:"category_id"`
	AccountID  int64  `json:"account_id"`
	Date       string `json:"date"` // YYYY-MM-DD
	Note       string `json:"note"`
}

func (h *hub) validateTxReq(uid int64, req *txReq) string {
	if req.Type != 1 && req.Type != 2 {
		return "类型无效"
	}
	if req.Amount <= 0 {
		return "金额必须大于 0"
	}
	if !validDate(req.Date) {
		return "日期格式错误"
	}
	var catType int
	err := h.db.QueryRow("SELECT type FROM categories WHERE id=? AND user_id=?", req.CategoryID, uid).Scan(&catType)
	if err != nil || catType != req.Type {
		return "分类无效"
	}
	var n int
	_ = h.db.QueryRow("SELECT COUNT(*) FROM accounts WHERE id=? AND user_id=?", req.AccountID, uid).Scan(&n)
	if n == 0 {
		return "账户无效"
	}
	return ""
}

type txItem struct {
	ID       int64  `json:"id"`
	Type     int    `json:"type"`
	Amount   int64  `json:"amount"`
	Date     string `json:"date"`
	Note     string `json:"note"`
	Category string `json:"category"`
	Account  string `json:"account"`
}

func (h *hub) handleListTx(w http.ResponseWriter, r *http.Request, uid int64) {
	q := r.URL.Query()
	month := q.Get("month")
	if month != "" && !validMonth(month) {
		errJSON(w, http.StatusBadRequest, "月份格式错误")
		return
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("size"))
	if size < 1 || size > 100 {
		size = 50
	}

	where := "t.user_id = ?"
	args := []any{uid}
	if month != "" {
		where += " AND substr(t.tx_date,1,7) = ?"
		args = append(args, month)
	}
	if t := q.Get("type"); t == "1" || t == "2" {
		where += " AND t.type = ?"
		args = append(args, t)
	}
	if cat := q.Get("category"); cat != "" {
		where += " AND t.category_id = ?"
		args = append(args, cat)
	}
	if s := q.Get("q"); s != "" {
		where += " AND t.note LIKE ? ESCAPE '\\'"
		args = append(args, "%"+escapeLike(s)+"%")
	}

	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM tx t WHERE "+where, args...).Scan(&total); err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	var sumIn, sumOut int64
	_ = h.db.QueryRow(`SELECT
			COALESCE(SUM(CASE WHEN t.type=2 THEN t.amount ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN t.type=1 THEN t.amount ELSE 0 END),0)
		FROM tx t WHERE `+where, args...).Scan(&sumIn, &sumOut)

	items := []txItem{}
	rows, err := h.db.Query(`SELECT t.id, t.type, t.amount, t.tx_date, t.note, c.name, a.name
		FROM tx t
		JOIN categories c ON c.id = t.category_id
		JOIN accounts a ON a.id = t.account_id
		WHERE `+where+`
		ORDER BY t.tx_date DESC, t.id DESC
		LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var it txItem
		if err := rows.Scan(&it.ID, &it.Type, &it.Amount, &it.Date, &it.Note, &it.Category, &it.Account); err != nil {
			continue
		}
		items = append(items, it)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "size": size,
		"sum_income": sumIn, "sum_expense": sumOut,
	})
}

func escapeLike(s string) string {
	r := ""
	for _, c := range s {
		switch c {
		case '%', '_', '\\':
			r += "\\" + string(c)
		default:
			r += string(c)
		}
	}
	return r
}

func (h *hub) handleCreateTx(w http.ResponseWriter, r *http.Request, uid int64) {
	var req txReq
	if err := decodeBody(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	if msg := h.validateTxReq(uid, &req); msg != "" {
		errJSON(w, http.StatusBadRequest, msg)
		return
	}
	res, err := h.db.Exec(
		"INSERT INTO tx(user_id, type, amount, category_id, account_id, tx_date, note) VALUES(?,?,?,?,?,?,?)",
		uid, req.Type, req.Amount, req.CategoryID, req.AccountID, req.Date, req.Note)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "保存失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *hub) handleUpdateTx(w http.ResponseWriter, r *http.Request, uid int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	var req txReq
	if err := decodeBody(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	if msg := h.validateTxReq(uid, &req); msg != "" {
		errJSON(w, http.StatusBadRequest, msg)
		return
	}
	res, err := h.db.Exec(`UPDATE tx SET type=?, amount=?, category_id=?, account_id=?, tx_date=?, note=?
		WHERE id=? AND user_id=?`,
		req.Type, req.Amount, req.CategoryID, req.AccountID, req.Date, req.Note, id, uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "保存失败")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		errJSON(w, http.StatusNotFound, "记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *hub) handleDeleteTx(w http.ResponseWriter, r *http.Request, uid int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	res, err := h.db.Exec("DELETE FROM tx WHERE id=? AND user_id=?", id, uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "删除失败")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		errJSON(w, http.StatusNotFound, "记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 分类 ----------

func (h *hub) handleListCategories(w http.ResponseWriter, r *http.Request, uid int64) {
	type cat struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Type int    `json:"type"`
		Used int    `json:"used"`
	}
	list := []cat{}
	rows, err := h.db.Query(`SELECT c.id, c.name, c.type,
			(SELECT COUNT(*) FROM tx t WHERE t.category_id = c.id) AS used
		FROM categories c WHERE c.user_id=? ORDER BY c.type, c.id`, uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c cat
		if rows.Scan(&c.ID, &c.Name, &c.Type, &c.Used) == nil {
			list = append(list, c)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *hub) handleCreateCategory(w http.ResponseWriter, r *http.Request, uid int64) {
	var req struct {
		Name string `json:"name"`
		Type int    `json:"type"`
	}
	if err := decodeBody(r, &req); err != nil || req.Name == "" || len(req.Name) > 20 || (req.Type != 1 && req.Type != 2) {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	res, err := h.db.Exec("INSERT INTO categories(user_id, name, type) VALUES(?,?,?)", uid, req.Name, req.Type)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "分类已存在或创建失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *hub) handleDeleteCategory(w http.ResponseWriter, r *http.Request, uid int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	var used int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM tx WHERE category_id=? AND user_id=?", id, uid).Scan(&used); err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if used > 0 {
		errJSON(w, http.StatusBadRequest, "该分类下有 "+strconv.Itoa(used)+" 笔流水，不能删除")
		return
	}
	if _, err := h.db.Exec("DELETE FROM categories WHERE id=? AND user_id=?", id, uid); err != nil {
		errJSON(w, http.StatusInternalServerError, "删除失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 账户 ----------

func (h *hub) handleListAccounts(w http.ResponseWriter, r *http.Request, uid int64) {
	type acc struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Used int    `json:"used"`
	}
	list := []acc{}
	rows, err := h.db.Query(`SELECT a.id, a.name,
			(SELECT COUNT(*) FROM tx t WHERE t.account_id = a.id) AS used
		FROM accounts a WHERE a.user_id=? ORDER BY a.id`, uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a acc
		if rows.Scan(&a.ID, &a.Name, &a.Used) == nil {
			list = append(list, a)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *hub) handleCreateAccount(w http.ResponseWriter, r *http.Request, uid int64) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil || req.Name == "" || len(req.Name) > 20 {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	res, err := h.db.Exec("INSERT INTO accounts(user_id, name) VALUES(?,?)", uid, req.Name)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "账户已存在或创建失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *hub) handleDeleteAccount(w http.ResponseWriter, r *http.Request, uid int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	var used int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM tx WHERE account_id=? AND user_id=?", id, uid).Scan(&used); err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if used > 0 {
		errJSON(w, http.StatusBadRequest, "该账户下有 "+strconv.Itoa(used)+" 笔流水，不能删除")
		return
	}
	if _, err := h.db.Exec("DELETE FROM accounts WHERE id=? AND user_id=?", id, uid); err != nil {
		errJSON(w, http.StatusInternalServerError, "删除失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 预算 ----------

func (h *hub) handleGetBudgets(w http.ResponseWriter, r *http.Request, uid int64) {
	month := r.URL.Query().Get("month")
	if !validMonth(month) {
		errJSON(w, http.StatusBadRequest, "月份格式错误")
		return
	}
	type budgetRow struct {
		CategoryID int64  `json:"category_id"`
		Name       string `json:"name"`
		Budget     int64  `json:"budget"`
		Spent      int64  `json:"spent"`
	}
	list := []budgetRow{}
	rows, err := h.db.Query(`SELECT c.id, c.name, COALESCE(b.amount, 0),
			(SELECT COALESCE(SUM(t.amount), 0) FROM tx t
				WHERE t.category_id = c.id AND t.user_id = c.user_id
				  AND substr(t.tx_date,1,7) = ? AND t.type = 1) AS spent
		FROM categories c
		LEFT JOIN budgets b ON b.category_id = c.id AND b.user_id = c.user_id AND b.month = ?
		WHERE c.user_id = ? AND c.type = 1
		ORDER BY c.id`, month, month, uid)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var b budgetRow
		if rows.Scan(&b.CategoryID, &b.Name, &b.Budget, &b.Spent) == nil {
			list = append(list, b)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"month": month, "items": list})
}

func (h *hub) handlePutBudget(w http.ResponseWriter, r *http.Request, uid int64) {
	var req struct {
		CategoryID int64  `json:"category_id"`
		Month      string `json:"month"`
		Amount     int64  `json:"amount"`
	}
	if err := decodeBody(r, &req); err != nil || !validMonth(req.Month) || req.Amount < 0 {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	var catType int
	err := h.db.QueryRow("SELECT type FROM categories WHERE id=? AND user_id=?", req.CategoryID, uid).Scan(&catType)
	if err != nil || catType != 1 {
		errJSON(w, http.StatusBadRequest, "只能给支出分类设预算")
		return
	}
	if req.Amount == 0 {
		_, _ = h.db.Exec("DELETE FROM budgets WHERE user_id=? AND category_id=? AND month=?", uid, req.CategoryID, req.Month)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	_, err = h.db.Exec(`INSERT INTO budgets(user_id, category_id, month, amount) VALUES(?,?,?,?)
		ON CONFLICT(user_id, category_id, month) DO UPDATE SET amount = excluded.amount`,
		uid, req.CategoryID, req.Month, req.Amount)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
