package main

import (
	"net/http"
	"time"
)

// GET /api/report?month=YYYY-MM
// 返回：当月收支总额、支出分类汇总、逐日支出、近 6 个月趋势
func (h *hub) handleReport(w http.ResponseWriter, r *http.Request, uid int64) {
	month := r.URL.Query().Get("month")
	if !validMonth(month) {
		errJSON(w, http.StatusBadRequest, "月份格式错误")
		return
	}

	var income, expense int64
	_ = h.db.QueryRow(`SELECT
			COALESCE(SUM(CASE WHEN type=2 THEN amount END),0),
			COALESCE(SUM(CASE WHEN type=1 THEN amount END),0)
		FROM tx WHERE user_id=? AND substr(tx_date,1,7)=?`, uid, month).Scan(&income, &expense)

	// 支出分类汇总
	type catRow struct {
		Name   string `json:"name"`
		Amount int64  `json:"amount"`
	}
	cats := []catRow{}
	if rows, err := h.db.Query(`SELECT c.name, SUM(t.amount)
		FROM tx t JOIN categories c ON c.id = t.category_id
		WHERE t.user_id=? AND substr(t.tx_date,1,7)=? AND t.type=1
		GROUP BY c.id ORDER BY 2 DESC`, uid, month); err == nil {
		for rows.Next() {
			var c catRow
			if rows.Scan(&c.Name, &c.Amount) == nil {
				cats = append(cats, c)
			}
		}
		rows.Close()
	}

	// 逐日支出
	type dayRow struct {
		Day    string `json:"day"`
		Expense int64 `json:"expense"`
	}
	days := []dayRow{}
	if rows, err := h.db.Query(`SELECT substr(tx_date,9,2), SUM(amount)
		FROM tx WHERE user_id=? AND substr(tx_date,1,7)=? AND type=1
		GROUP BY tx_date ORDER BY tx_date`, uid, month); err == nil {
		for rows.Next() {
			var d dayRow
			if rows.Scan(&d.Day, &d.Expense) == nil {
				days = append(days, d)
			}
		}
		rows.Close()
	}

	// 近 6 个月趋势（含当月）
	end, _ := time.Parse("2006-01", month)
	months := make([]string, 6)
	for i := 5; i >= 0; i-- {
		months[5-i] = end.AddDate(0, -i, 0).Format("2006-01")
	}
	spent := map[string][2]int64{} // {income, expense}
	if rows, err := h.db.Query(`SELECT substr(tx_date,1,7),
			COALESCE(SUM(CASE WHEN type=2 THEN amount END),0),
			COALESCE(SUM(CASE WHEN type=1 THEN amount END),0)
		FROM tx WHERE user_id=? AND substr(tx_date,1,7) BETWEEN ? AND ?
		GROUP BY substr(tx_date,1,7)`, uid, months[0], months[5]); err == nil {
		for rows.Next() {
			var m string
			var out, in int64
			if rows.Scan(&m, &in, &out) == nil {
				spent[m] = [2]int64{in, out}
			}
		}
		rows.Close()
	}
	trend := make([]map[string]any, 0, 6)
	for _, m := range months {
		v := spent[m]
		trend = append(trend, map[string]any{"month": m, "income": v[0], "expense": v[1]})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"month":       month,
		"income":      income,
		"expense":     expense,
		"net":         income - expense,
		"by_category": cats,
		"daily":       days,
		"trend":       trend,
	})
}
