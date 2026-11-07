package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- settings ----

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

// publicSettings is what one person sees: their settings with the secrets
// masked and, per Pocket app, the mode in effect (<app>_mode), whether it
// works and how (<app>_via: core | direct | "") and why not (<app>_detail).
func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	out := map[string]any{
		"core_configured": s.core.Configured(), "core_registered": s.coreStatus()["registered"],
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart,
		"currency": st.Currency, "due_day": st.DueDay, "grace_days": st.GraceDays, "notice_days": st.NoticeDays, "remind_days": st.RemindDays, "ending_days": st.EndingDays,
		"money_book": st.MoneyBook, "money_match": st.MoneyMatch, "money_account": st.MoneyAccount, "money_income_cat": st.MoneyIncomeCat, "money_expense_cat": st.MoneyExpenseCat,
		"money_match_days": st.MoneyMatchDays, "money_match_pct": st.MoneyMatchMargin,
		"repair_tasks": st.RepairTasks, "calendar_sync": st.CalendarSync, "home_log": st.HomeLog, "mail_from": st.MailFrom, "repair_form": st.RepairForm,
		"reminder_text": st.ReminderText, "invoice_rent": st.InvoiceRent, "ai_help": st.AIHelp,
		"notify_digest": st.NotifyDigest, "notify_hour": st.NotifyHour,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"ai_provider": st.AIProvider, "ai_pocket_model": st.AIPocketModel, "openai_api_key": mask(st.OpenAIKey), "openai_model": st.OpenAIModel, "openai_base_url": st.OpenAIBaseURL,
		"setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL, "demo": s.cfg.DemoData,
	}
	for _, a := range pocketApps {
		mode, via, detail := s.connStatus(ctx, u, st, a)
		out[a.slug+"_mode"], out[a.slug+"_via"], out[a.slug+"_detail"] = mode, via, detail
		out[a.slug+"_configured"] = via != ""
		_, url, token, public := connSettings(st, a)
		out[a.slug+"_url"], out[a.slug+"_token"], out[a.slug+"_public_url"] = url, mask(token), public
		if public == "" && via != "" {
			public = s.resolvePublic(ctx, a.slug)
		}
		out[a.slug+"_open_url"] = public
	}
	return out
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), userFrom(r.Context()), st))
}

// handlePutSettings saves the caller's personal settings. The box-wide title
// is only taken from an admin.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r.Context())
	isAdmin := me.Role == "admin"
	cur, err := s.store.GetSettings(r.Context(), me.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	raw := map[string]any{}
	var in store.Settings
	keys, err := decodeKeys(r, &raw)
	if err != nil || remarshal(raw, &in) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	bad := func(msg string) { writeErr(w, http.StatusBadRequest, msg) }
	if keys["title"] && isAdmin {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["week_start"] && (in.WeekStart == 1 || in.WeekStart == 7 || in.WeekStart == 6) {
		cur.WeekStart = in.WeekStart
	}
	if keys["notify_hour"] {
		if in.NotifyHour < 0 || in.NotifyHour > 23 {
			bad("the hour is 0 to 23")
			return
		}
		cur.NotifyHour = in.NotifyHour
	}
	secret := func(k string, dst *string, v string) {
		if keys[k] && !strings.Contains(v, "••") {
			*dst = strings.TrimSpace(v)
		}
	}
	url := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	text := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimSpace(v)
		}
	}
	boolIn := func(k string, dst *bool, v bool) {
		if keys[k] {
			*dst = v
		}
	}
	if keys["currency"] {
		c := strings.ToUpper(strings.TrimSpace(in.Currency))
		if c == "KM" {
			c = "BAM"
		}
		if !reCur.MatchString(c) {
			bad("the currency is a three-letter code like EUR")
			return
		}
		cur.Currency = c
	}
	num := func(k string, dst *int, v, lo, hi int, msg string) bool {
		if !keys[k] {
			return true
		}
		if v < lo || v > hi {
			bad(msg)
			return false
		}
		*dst = v
		return true
	}
	if !num("due_day", &cur.DueDay, in.DueDay, 1, 28, "rent is due on a day from 1 to 28") ||
		!num("grace_days", &cur.GraceDays, in.GraceDays, 0, 60, "rent may be 0 to 60 days late before it counts as late") ||
		!num("notice_days", &cur.NoticeDays, in.NoticeDays, 0, 365, "notice is 0 to 365 days") ||
		!num("remind_days", &cur.RemindDays, in.RemindDays, 0, 30, "say rent is coming 0 to 30 days ahead") ||
		!num("ending_days", &cur.EndingDays, in.EndingDays, 7, 365, "a lease ends soon 7 to 365 days ahead") ||
		!num("money_match_days", &cur.MoneyMatchDays, in.MoneyMatchDays, 1, 31, "a payment may be 1 to 31 days from the due day") ||
		!num("money_match_pct", &cur.MoneyMatchMargin, in.MoneyMatchMargin, 0, 50, "the amount may be off by 0 to 50 percent") {
		return
	}
	for _, k := range []struct {
		key string
		dst *int64
		v   int64
	}{{"money_account", &cur.MoneyAccount, in.MoneyAccount}, {"money_income_cat", &cur.MoneyIncomeCat, in.MoneyIncomeCat}, {"money_expense_cat", &cur.MoneyExpenseCat, in.MoneyExpenseCat}} {
		if keys[k.key] && k.v >= 0 {
			*k.dst = k.v
		}
	}
	boolIn("money_book", &cur.MoneyBook, in.MoneyBook)
	boolIn("money_match", &cur.MoneyMatch, in.MoneyMatch)
	boolIn("repair_tasks", &cur.RepairTasks, in.RepairTasks)
	boolIn("calendar_sync", &cur.CalendarSync, in.CalendarSync)
	boolIn("home_log", &cur.HomeLog, in.HomeLog)
	boolIn("invoice_rent", &cur.InvoiceRent, in.InvoiceRent)
	text("mail_from", &cur.MailFrom, in.MailFrom)
	text("repair_form", &cur.RepairForm, in.RepairForm)
	if keys["reminder_text"] {
		cur.ReminderText = clip(strings.TrimSpace(in.ReminderText), 4000)
		if cur.ReminderText == "" {
			cur.ReminderText = store.DefaultReminder
		}
	}
	boolIn("ai_help", &cur.AIHelp, in.AIHelp)
	boolIn("notify_digest", &cur.NotifyDigest, in.NotifyDigest)
	text("notify_url", &cur.NotifyURL, in.NotifyURL)
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	// each Pocket app: mode, URL, token, public URL
	inMap := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			inMap[k] = sv
		}
	}
	for _, a := range pocketApps {
		c := cur.Conns[a.slug]
		if v := inMap[a.slug+"_mode"]; keys[a.slug+"_mode"] && (v == modeCore || v == modeDirect || v == modeOff) {
			c.Mode = v
		}
		url(a.slug+"_url", &c.URL, inMap[a.slug+"_url"])
		secret(a.slug+"_token", &c.Token, inMap[a.slug+"_token"])
		url(a.slug+"_public_url", &c.PublicURL, inMap[a.slug+"_public_url"])
		if c != (store.Conn{}) {
			cur.Conns[a.slug] = c
		}
	}
	if keys["ai_provider"] {
		if !validAIProvider(in.AIProvider) {
			bad("AI is automatic, Pocket AI or OpenAI")
			return
		}
		cur.AIProvider = in.AIProvider
	}
	if keys["ai_pocket_model"] {
		if !validPocketModel(in.AIPocketModel) {
			bad("unknown model")
			return
		}
		cur.AIPocketModel = in.AIPocketModel
	}
	secret("openai_api_key", &cur.OpenAIKey, in.OpenAIKey)
	text("openai_model", &cur.OpenAIModel, in.OpenAIModel)
	url("openai_base_url", &cur.OpenAIBaseURL, in.OpenAIBaseURL)
	if err := s.store.SaveUserSettings(r.Context(), me.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if isAdmin && keys["title"] {
		if err := s.store.SaveBoxSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	s.forgetStatus(me.ID)
	for k := range keys {
		if isModeKey(k) {
			s.reportModesSoon()
			break
		}
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), me, cur))
}

// handleIntegrationTest checks a connection with the caller's own saved
// settings (or the URL/key typed into the form).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which  string `json:"which"`
		URL    string `json:"url"`
		Key    string `json:"key"`
		Format string `json:"format"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := actorID(r.Context())
	st, _ := s.store.GetSettings(r.Context(), uid)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	keyOr := func(saved string) string {
		if strings.Contains(in.Key, "••") || in.Key == "" {
			return saved
		}
		return in.Key
	}
	urlOr := func(saved string) string {
		if strings.TrimSpace(in.URL) == "" {
			return saved
		}
		return strings.TrimRight(strings.TrimSpace(in.URL), "/")
	}
	var msg string
	var err error
	if a, ok := appBySlug(in.Which); ok {
		mode, savedURL, savedTok, _ := connSettings(st, a)
		typed := in.Key != "" && !strings.Contains(in.Key, "••")
		var conn *siblingConn
		if tok := keyOr(savedTok); tok != "" && (typed || s.connMode(mode, savedTok) == modeDirect) {
			base := urlOr(savedURL)
			if base == "" {
				base = s.resolveSibling(ctx, a.slug)
			}
			conn = &siblingConn{client: pocket.New(base, tok, 15*time.Second), via: modeDirect}
		} else {
			conn, err = s.clients(ctx, uid).conn(ctx, a, a.scopes...)
		}
		if err == nil {
			how := "direct connection"
			if conn.via == modeCore {
				how = "through Pocket Core"
			}
			var v string
			if v, err = conn.client.Health(ctx); err == nil {
				msg = fmt.Sprintf("%s %s, %s", a.name, v, how)
			}
		}
	} else if in.Which == "openai" {
		c := ai.New(urlOr(st.OpenAIBaseURL), keyOr(st.OpenAIKey), st.OpenAIModel, 20*time.Second)
		var ids []string
		if ids, err = c.Models(ctx); err == nil {
			msg = fmt.Sprintf("The key works (%d models)", len(ids))
		}
	} else if in.Which == "notify" {
		f := in.Format
		if f == "" {
			f = st.NotifyFormat
		}
		n := notify.New(urlOr(st.NotifyURL), f)
		err = n.Send(ctx, notify.Message{Title: "Pocket Rentals", Body: notifyTestBody, Priority: "info", Tags: []string{"memo"}})
		msg = "Test message sent via " + n.Format
	} else {
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// ---- system / backup / export ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.CountsFor(r.Context(), actorID(r.Context())),
	})
}

// handleBackup downloads the whole database, an admin-only box operation.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-rentals-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, dst)
}
