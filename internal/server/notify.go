package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// pushes runs every minute; once a day, at each person's own hour, it makes
// the rent that is due, says what needs them (when they want that), and
// tells the box for Pocket Flows: rentals.rent_due, rentals.rent_late,
// rentals.lease_ending.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.ensureUser(ctx, uid, true)
		s.dailyCheck(ctx, uid, st, now, st.NotifyDigest)
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// dailyCheck builds one person's morning message and the day's events.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time, send bool) (string, bool) {
	b, err := s.loadBook(ctx, uid, false)
	if err != nil {
		return "", false
	}
	t := now.Format("2006-01-02")
	var dueToday, dueSoon, late []string
	var ending []string
	lateCount := 0
	for _, l := range b.leases {
		if l.Status != store.LeaseActive {
			continue
		}
		for _, c := range b.charges[l.ID] {
			switch {
			case c.State == "late":
				if s.once(ctx, fmt.Sprintf("late:%d:%d", uid, c.ID)) {
					s.publish(ctx, uid, "rentals.rent_late", "charge:"+itoa(c.ID), map[string]any{"lease_id": l.ID, "place": place(l.UnitName, l.PropertyName),
						"tenants": tenantNames(l.Tenants), "title": c.Title, "due": c.DueDate, "owed": c.Amount - c.Paid, "currency": c.Currency, "url": s.link("/leases/" + itoa(l.ID))})
				}
			case (c.State == "due" || c.State == "part") && c.DueDate == t:
				dueToday = append(dueToday, fmt.Sprintf("%s (%s)", place(l.UnitName, l.PropertyName), fmtMoney(c.Amount-c.Paid, c.Currency)))
			case c.State == "upcoming" && c.DueDate <= store.AddDays(t, max(st.RemindDays, 0)):
				dueSoon = append(dueSoon, place(l.UnitName, l.PropertyName))
			}
		}
		if l.Overdue > 0 {
			lateCount++
			late = append(late, fmt.Sprintf("%s owes %s", firstNonEmpty(tenantNames(l.Tenants), place(l.UnitName, l.PropertyName)), fmtMoney(l.Overdue, l.Currency)))
		}
		if l.DaysLeft != nil {
			d := *l.DaysLeft
			for _, mark := range []int{st.EndingDays, 30, 7, 0} {
				if d == mark && mark >= 0 {
					ending = append(ending, fmt.Sprintf("%s ends %s", place(l.UnitName, l.PropertyName), whenWords(d)))
					if s.once(ctx, fmt.Sprintf("ending:%d:%d:%d", uid, l.ID, mark)) {
						ev := s.leaseEvent(l)
						ev["days_left"] = d
						s.publish(ctx, uid, "rentals.lease_ending", "lease:"+itoa(l.ID)+":"+itoa(int64(mark)), ev)
					}
					break
				}
			}
		}
	}
	urgent := 0
	for _, rq := range b.requests {
		if rq.Open() && rq.Priority == "urgent" {
			urgent++
		}
	}
	insp, _ := s.store.ListInspections(ctx, uid, 0, 0)
	var inspToday []string
	for _, i := range insp {
		if i.Status == "planned" && i.Date == t {
			inspToday = append(inspToday, place(i.UnitName, i.PropertyName))
		}
	}
	var lines []string
	if len(dueToday) > 0 {
		lines = append(lines, "Rent due today: "+strings.Join(dueToday[:min(len(dueToday), 4)], "; ")+".")
	}
	if len(dueSoon) > 0 {
		lines = append(lines, fmt.Sprintf("%d more %s due in the next %d days.", len(dueSoon), plural(len(dueSoon), "rent is", "rents are"), st.RemindDays))
	}
	if lateCount > 0 {
		lines = append(lines, "Late: "+strings.Join(late[:min(len(late), 3)], "; ")+".")
	}
	if len(ending) > 0 {
		lines = append(lines, strings.Join(ending, "; ")+".")
	}
	if urgent > 0 {
		lines = append(lines, fmt.Sprintf("%d urgent %s waiting.", urgent, plural(urgent, "repair", "repairs")))
	}
	if len(inspToday) > 0 {
		lines = append(lines, "Inspection today: "+strings.Join(inspToday, ", ")+".")
	}
	if len(dueToday)+len(dueSoon) > 0 {
		s.publish(ctx, uid, "rentals.rent_due", "rent:"+t, map[string]any{"date": t, "today": len(dueToday), "soon": len(dueSoon), "late": lateCount, "url": s.link("/rent")})
	}
	if len(lines) == 0 {
		return "", false
	}
	body := strings.Join(lines, "\n")
	if !send || !s.push(ctx, uid, st, "Your rentals today", body, s.link("/")) {
		return body, false
	}
	return body, true
}

func whenWords(d int) string {
	switch {
	case d < 0:
		return fmt.Sprintf("%d days ago", -d)
	case d == 0:
		return "today"
	case d == 1:
		return "tomorrow"
	}
	return fmt.Sprintf("in %d days", d)
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"house"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Rentals", Body: notifyTestBody, Priority: "info", Tags: []string{"house"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow builds today's message now (and sends it when asked).
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	s.ensureUser(ctx, uid, true)
	body, sent := s.dailyCheck(ctx, uid, st, time.Now(), queryBool(r, "send"))
	if body == "" {
		body = "Nothing needs you today."
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": body, "sent": sent})
}

const notifyTestBody = "Notifications work. Each morning, if you want it, a note says which rent is due or late, which leases end soon and which repairs are urgent."
