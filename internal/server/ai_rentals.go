package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- Pocket AI for renting ----

const aiSystem = `You help someone who rents out a home or a few flats: a private landlord, not a company. Write plainly and kindly, like a sensible neighbour who knows the business. No jargon, no legal threats, no emoji. Use the language the notes are written in; when there are none, English.`

// leaseBrief is a lease in words for the AI.
func (s *Server) leaseBrief(ctx context.Context, uid, id int64) (string, *store.Lease) {
	l, cs, ps, err := s.leaseFull(ctx, uid, id)
	if err != nil {
		return "", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Place: %s\nTenants: %s\nRent: %s %s, due on day %d\nRuns: %s to %s (notice %d days)\n", place(l.UnitName, l.PropertyName), firstNonEmpty(tenantNames(l.Tenants), "unknown"),
		fmtMoney(l.Rent, l.Currency), l.Frequency, l.DueDay, l.StartDate, firstNonEmpty(l.EndDate, "until notice"), l.NoticeDays)
	fmt.Fprintf(&b, "Owed now: %s (late: %s)\nNext rent day: %s\nToday: %s\n", fmtMoney(max(l.Balance, 0), l.Currency), fmtMoney(l.Overdue, l.Currency), firstNonEmpty(l.NextDue, "-"), today())
	b.WriteString("Recent charges:\n")
	for i := len(cs) - 1; i >= 0 && i >= len(cs)-6; i-- {
		fmt.Fprintf(&b, "- %s %s %s (%s)\n", cs[i].DueDate, cs[i].Title, fmtMoney(cs[i].Amount, cs[i].Currency), cs[i].State)
	}
	b.WriteString("Recent payments:\n")
	for i := len(ps) - 1; i >= 0 && i >= len(ps)-4; i-- {
		fmt.Fprintf(&b, "- %s %s\n", ps[i].Date, fmtMoney(ps[i].Amount, ps[i].Currency))
	}
	if l.Notes != "" {
		b.WriteString("Notes: " + clip(l.Notes, 1500) + "\n")
	}
	return b.String(), l
}

func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return
	}
	var in struct {
		Want         string `json:"want"` // reminder | late | ending | inspection | repair
		LeaseID      int64  `json:"lease_id"`
		InspectionID int64  `json:"inspection_id"`
		RequestID    int64  `json:"request_id"`
		Hint         string `json:"hint"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return
	}
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-time.After(20 * time.Second):
		writeErr(w, http.StatusServiceUnavailable, "the AI is busy, try again in a moment")
		return
	}
	sign := ""
	if me := userFrom(ctx); me != nil {
		sign = me.Name
	}
	hint := ""
	if h := strings.TrimSpace(in.Hint); h != "" {
		hint = " It should also say: " + clip(h, 1000)
	}
	if sign != "" {
		hint += " Sign it " + sign + "."
	}
	email := func(task, brief string) {
		var out struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		err := cl.JSON(ctx, aiSystem+` Answer as JSON {"subject": "...", "body": "..."}. The body is plain text with blank lines between paragraphs, at most 140 words.`, brief+"\n"+task+hint, &out)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"subject": clip(strings.TrimSpace(out.Subject), 200), "body": strings.TrimSpace(out.Body)})
	}
	switch in.Want {
	case "reminder", "late", "ending":
		brief, l := s.leaseBrief(ctx, uid, in.LeaseID)
		if l == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		task := map[string]string{
			"reminder": "Write a short, friendly reminder to the tenants that the next rent is due soon.",
			"late":     "The rent is late. Write a short, polite but clear note to the tenants: what is owed and since when, and ask them to pay or to get in touch if something is wrong.",
			"ending":   "The lease ends soon. Write a short note to the tenants asking whether they would like to stay (and on what terms you would renew) or confirming the move-out day and what happens with the deposit and the keys.",
		}[in.Want]
		email(task, brief)
	case "inspection":
		i, err := s.store.GetInspection(ctx, uid, in.InspectionID)
		if err != nil {
			s.fail(w, err, "inspection")
			return
		}
		sub := &subject{inspection: i}
		before := s.earlierInspection(ctx, uid, i)
		text := s.subjectMarkdown(ctx, uid, sub)
		if before != nil {
			text += "\n\nEarlier (" + inspectionWords[before.Kind] + " on " + before.Date + "):\n" + s.subjectMarkdown(ctx, uid, &subject{inspection: before})
		}
		ask := "Sum up this inspection in three to six short lines: what is fine, what needs fixing, and anything urgent."
		if i.Kind == "move_out" {
			ask = "Compare moving out with how the place was before. List what got worse beyond normal wear (with a fair cost to keep from the deposit if any), and what is just normal wear. Be fair to the tenant. Short lines."
		}
		txt, err := cl.Complete(ctx, aiSystem, ask+"\n\n"+text, false)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
	case "repair":
		rq, err := s.store.GetRequest(ctx, uid, in.RequestID)
		if err != nil {
			s.fail(w, err, "repair")
			return
		}
		var out struct {
			Priority string `json:"priority"`
			Trade    string `json:"trade"`
			Steps    string `json:"steps"`
		}
		prompt := fmt.Sprintf("A repair at %s.\nWhat: %s\nDetails: %s", place(rq.UnitName, rq.PropertyName), rq.Title, clip(rq.Details, 3000))
		err = cl.JSON(ctx, aiSystem+` Answer as JSON {"priority": "low|normal|urgent", "trade": "...", "steps": "..."}. priority: urgent only for safety, water, heating in winter, no power or a door that does not lock. trade: who usually fixes this (plumber, electrician, handyman…). steps: two or three short lines on what to do first, including anything the tenant can do safely now.`, prompt, &out)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if !oneOf(out.Priority, store.Priorities) {
			out.Priority = "normal"
		}
		writeJSON(w, http.StatusOK, map[string]any{"priority": out.Priority, "trade": clip(strings.TrimSpace(out.Trade), 80), "steps": clip(strings.TrimSpace(out.Steps), 1200)})
	default:
		writeErr(w, http.StatusBadRequest, "ask for reminder, late, ending, inspection or repair")
	}
}
