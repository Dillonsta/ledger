package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 30 * 24 * time.Hour

type authService struct {
	key []byte
}

// newAuth 加载或生成 HMAC 签名密钥（持久化在数据目录，重启不掉线）
func newAuth(dataDir string) (*authService, error) {
	p := filepath.Join(dataDir, "token.key")
	b, err := os.ReadFile(p)
	if err == nil && len(b) == 32 {
		return &authService{key: b}, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	b = make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return nil, err
	}
	return &authService{key: b}, nil
}

func (a *authService) sign(s string) string {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *authService) issue(userID int64) (string, error) {
	payload, _ := json.Marshal(map[string]int64{
		"uid": userID,
		"exp": time.Now().Add(tokenTTL).Unix(),
	})
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	return b64 + "." + a.sign(b64), nil
}

var errBadToken = errors.New("invalid token")

func (a *authService) verify(token string) (int64, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return 0, errBadToken
	}
	if !hmac.Equal([]byte(a.sign(parts[0])), []byte(parts[1])) {
		return 0, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errBadToken
	}
	var p struct {
		Uid int64 `json:"uid"`
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, errBadToken
	}
	if time.Now().Unix() > p.Exp {
		return 0, errBadToken
	}
	return p.Uid, nil
}

// requireAuth 中间件：Authorization: Bearer 或 cookie
func (h *hub) requireAuth(next func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			token = strings.TrimPrefix(ah, "Bearer ")
		}
		if token == "" {
			if c, err := r.Cookie("ledger_token"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			errJSON(w, http.StatusUnauthorized, "未登录")
			return
		}
		uid, err := h.auth.verify(token)
		if err != nil {
			errJSON(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		next(w, r, uid)
	}
}

func (h *hub) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil || req.Username == "" || req.Password == "" {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	var id int64
	var hash string
	err := h.db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", req.Username).Scan(&id, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		errJSON(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	token, err := h.auth.issue(id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "ledger_token", Value: token, Path: "/",
		MaxAge: int(tokenTTL.Seconds()), SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user_id": id, "username": req.Username})
}

func (h *hub) handleMe(w http.ResponseWriter, r *http.Request, uid int64) {
	var name string
	if err := h.db.QueryRow("SELECT username FROM users WHERE id=?", uid).Scan(&name); err != nil {
		errJSON(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": uid, "username": name})
}

func (h *hub) handleChangePassword(w http.ResponseWriter, r *http.Request, uid int64) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeBody(r, &req); err != nil {
		errJSON(w, http.StatusBadRequest, "参数错误")
		return
	}
	if len(req.NewPassword) < 6 {
		errJSON(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	var hash string
	if err := h.db.QueryRow("SELECT password_hash FROM users WHERE id=?", uid).Scan(&hash); err != nil {
		errJSON(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		errJSON(w, http.StatusBadRequest, "原密码错误")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if _, err := h.db.Exec("UPDATE users SET password_hash=? WHERE id=?", string(newHash), uid); err != nil {
		errJSON(w, http.StatusInternalServerError, "保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
