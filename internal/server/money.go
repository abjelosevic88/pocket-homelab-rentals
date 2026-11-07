package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-rentals/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-rentals/internal/store"
)

// ---- Pocket Money ----
//
// Rent that came in can be booked as income on one of the person's
// accounts, and each cost of a place as an expense — as the person, through
// their own link, never twice (Money keeps external ids idempotent). Rent
// already in the bank is found the other way round: incoming transactions
// that look like a lease's rent (about the amount, near the due day, the
// tenant's name in the payee) are offered, and one tap links them.

type moneyResult struct {
	OK      bool   `json:"ok"`
	TxID    int64  `json:"tx_id,omitempty"`
	Account string `json:"account,omitempty"`
	URL     string `json:"url,omitempty"`
	Via     string `json:"via,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

func pickAccount(accts []pocket.Account, want, setting int64) *pocket.Account {
	for _, id := range []int64{want, setting} {
		if id <= 0 {
			continue
		}
		for i := range accts {
			if accts[i].ID == id {
				return &accts[i]
			}
		}
	}
	for i := range accts {
		if accts[i].Everyday {
			return &accts[i]
		}
	}
	if len(accts) > 0 {
		return &accts[0]
	}
	return nil
}

// book creates one transaction and returns what happened.
func (s *Server) book(ctx context.Context, uid int64, tx pocket.Tx, accountID int64) moneyResult {
	s.moneyMu.Lock()
	defer s.moneyMu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	c := s.clients(cctx, uid)
	conn, err := c.conn(cctx, appMoney, scopeAccountsRead, scopeTxWrite)
	if err != nil {
		return moneyResult{Error: err.Error()}
	}
	accts, err := conn.client.Accounts(cctx)
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	acct := pickAccount(accts, accountID, c.st.MoneyAccount)
	if acct == nil {
		return moneyResult{Error: "Pocket Money has no account you can book on", Via: conn.via}
	}
	tx.AccountID = acct.ID
	if tx.CategoryID == 0 {
		cat := c.st.MoneyExpenseCat
		if tx.Kind == "income" {
			cat = c.st.MoneyIncomeCat
		}
		if cat > 0 {
			if cats, err := conn.client.Categories(cctx, acct.ID, tx.Kind); err == nil {
				for _, ct := range cats {
					if ct.ID == cat {
						tx.CategoryID = cat
					}
				}
			}
		}
	}
	id, err := conn.client.CreateTx(cctx, tx)
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	return moneyResult{OK: true, TxID: id, Account: acct.Name, Via: conn.via, URL: strings.TrimRight(conn.public, "/") + "/transactions/" + itoa(id)}
}

func (s *Server) moneyTags(ctx context.Context, uid, propertyID int64, kind string) []string {
	tags := []string{kind}
	if p, err := s.store.GetProperty(ctx, uid, propertyID); err == nil && p.MoneyTag != "" {
		tags = append(tags, p.MoneyTag)
	}
	return tags
}

func (s *Server) paymentTx(ctx context.Context, uid int64, p *store.Payment, l *store.Lease) pocket.Tx {
	who := tenantNames(l.Tenants)
	return pocket.Tx{Kind: "income", Date: p.Date, Amount: p.Amount, Currency: p.Currency, Description: "Rent — " + place(l.UnitName, l.PropertyName),
		Payee: firstNonEmpty(who, "Tenant"), Tags: s.moneyTags(ctx, uid, l.PropertyID, "rent"),
		Notes: joinNonEmpty(" ", "Pocket Rentals:", p.Note, s.link("/leases/"+itoa(l.ID))), ExternalID: fmt.Sprintf("rentals:payment:%d", p.ID)}
}

// bookPayment books a payment as income and remembers the transaction.
func (s *Server) bookPayment(ctx context.Context, uid int64, p *store.Payment, l *store.Lease, accountID int64) moneyResult {
	if p.MoneyTxRef != "" {
		return moneyResult{OK: true, Skipped: true}
	}
	res := s.book(ctx, uid, s.paymentTx(ctx, uid, p, l), accountID)
	if res.OK {
		p.MoneyTxRef = itoa(res.TxID)
		_ = s.store.UpdatePayment(ctx, p)
	}
	return res
}

// syncPaymentMoney carries an edited payment over to its transaction.
func (s *Server) syncPaymentMoney(ctx context.Context, uid int64, p *store.Payment, l *store.Lease) moneyResult {
	return s.resync(ctx, uid, p.MoneyTxRef, s.paymentTx(ctx, uid, p, l), func() { p.MoneyTxRef = ""; _ = s.store.UpdatePayment(ctx, p) })
}

func (s *Server) resync(ctx context.Context, uid int64, ref string, tx pocket.Tx, gone func()) moneyResult {
	id, _ := strconv.ParseInt(ref, 10, 64)
	if id == 0 {
		return moneyResult{Skipped: true}
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxWrite)
	if err != nil {
		return moneyResult{Error: err.Error()}
	}
	err = conn.client.UpdateTx(cctx, id, tx)
	if errors.Is(err, pocket.ErrNotFound) {
		gone()
		return moneyResult{OK: true, Skipped: true}
	}
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	return moneyResult{OK: true, TxID: id, Via: conn.via}
}

// unbookTx removes a transaction Rentals booked.
func (s *Server) unbookTx(ctx context.Context, uid int64, ref string) moneyResult {
	id, _ := strconv.ParseInt(ref, 10, 64)
	if id == 0 {
		return moneyResult{Skipped: true}
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxWrite)
	if err != nil {
		return moneyResult{Error: err.Error()}
	}
	if err := conn.client.DeleteTx(cctx, id); err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	return moneyResult{OK: true, Via: conn.via}
}

var expenseWords = map[string]string{"repairs": "Repair", "insurance": "Insurance", "tax": "Property tax", "utilities": "Bills", "loan_interest": "Loan interest",
	"building_fee": "Building fee", "management": "Management", "cleaning": "Cleaning", "furniture": "Furniture", "supplies": "Supplies", "legal": "Legal and fees", "other": "Cost"}

func (s *Server) expenseTx(ctx context.Context, uid int64, e *store.Expense) pocket.Tx {
	desc := expenseWords[e.Category]
	if e.Note != "" && e.Category != "other" {
		desc += ": " + e.Note
	} else if e.Note != "" {
		desc = e.Note
	}
	return pocket.Tx{Kind: "expense", Date: e.Date, Amount: e.Amount, Currency: e.Currency, Description: clip(desc+" — "+e.PropertyName, 200), Payee: e.Payee,
		Tags: s.moneyTags(ctx, uid, e.PropertyID, "rental"), Notes: "Pocket Rentals: " + s.link("/properties/"+itoa(e.PropertyID)), ExternalID: fmt.Sprintf("rentals:expense:%d", e.ID)}
}

// bookExpense books an expense and remembers the transaction.
func (s *Server) bookExpense(ctx context.Context, uid int64, e *store.Expense, accountID int64) moneyResult {
	if e.MoneyTxRef != "" {
		return moneyResult{OK: true, Skipped: true}
	}
	if e.PropertyName == "" {
		if p, err := s.store.GetProperty(ctx, uid, e.PropertyID); err == nil {
			e.PropertyName = p.Name
		}
	}
	res := s.book(ctx, uid, s.expenseTx(ctx, uid, e), accountID)
	if res.OK {
		e.MoneyTxRef = itoa(res.TxID)
		_ = s.store.UpdateExpense(ctx, e)
	}
	return res
}

func (s *Server) syncExpenseMoney(ctx context.Context, uid int64, e *store.Expense) moneyResult {
	return s.resync(ctx, uid, e.MoneyTxRef, s.expenseTx(ctx, uid, e), func() { e.MoneyTxRef = ""; _ = s.store.UpdateExpense(ctx, e) })
}

// bookDepositBack books a deposit given back as money out.
func (s *Server) bookDepositBack(ctx context.Context, uid int64, l *store.Lease, amount int64, date string) moneyResult {
	tx := pocket.Tx{Kind: "expense", Date: date, Amount: amount, Currency: l.Currency, Description: "Deposit back — " + place(l.UnitName, l.PropertyName),
		Payee: tenantNames(l.Tenants), Tags: s.moneyTags(ctx, uid, l.PropertyID, "deposit"), Notes: "Pocket Rentals: " + s.link("/leases/"+itoa(l.ID)),
		ExternalID: fmt.Sprintf("rentals:deposit:%d", l.ID)}
	return s.book(ctx, uid, tx, 0)
}

func (s *Server) handleBookPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPayment(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "payment")
		return
	}
	l, err := s.store.GetLease(ctx, uid, p.LeaseID)
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	var in struct {
		AccountID int64 `json:"account_id"`
	}
	_ = decode(r, &in)
	writeJSON(w, http.StatusOK, s.bookPayment(ctx, uid, p, l, in.AccountID))
}

func (s *Server) handleBookExpense(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetExpense(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "expense")
		return
	}
	var in struct {
		AccountID int64 `json:"account_id"`
	}
	_ = decode(r, &in)
	writeJSON(w, http.StatusOK, s.bookExpense(ctx, uid, e, in.AccountID))
}

// ---- rent already in the bank ----

type suggestion struct {
	TxID        string `json:"tx_id"`
	Date        string `json:"date"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Payee       string `json:"payee"`
	Description string `json:"description"`
	Account     string `json:"account"`
	URL         string `json:"url"`
	LeaseID     int64  `json:"lease_id"`
	LeaseTitle  string `json:"lease_title"`
	Owed        int64  `json:"owed"`
	Why         string `json:"why"`
	Score       int    `json:"score"`
}

// nameHit says whether a payee or description has a tenant's name in it.
func nameHit(text string, ts []*store.Tenant) bool {
	text = strings.ToLower(text)
	for _, t := range ts {
		for _, part := range strings.Fields(strings.ToLower(t.Name)) {
			if len([]rune(part)) >= 3 && strings.Contains(text, part) {
				return true
			}
		}
	}
	return false
}

// suggestions looks at the last weeks of money in and pairs each
// transaction with the lease it most likely pays.
func (s *Server) suggestions(ctx context.Context, uid int64, onlyLease int64) ([]suggestion, string, error) {
	st := s.settingsOf(ctx, uid)
	c := s.clients(ctx, uid)
	as := c.status(ctx, appMoney)
	if as.Via == "" || !as.Can(scopeTxRead) {
		return nil, firstNonEmpty(as.Fix, as.Detail, "Pocket Money is not connected"), nil
	}
	conn, err := c.conn(ctx, appMoney, scopeTxRead)
	if err != nil {
		return nil, err.Error(), nil
	}
	b, err := s.loadBook(ctx, uid, false)
	if err != nil {
		return nil, "", err
	}
	t := today()
	days := max(st.MoneyMatchDays, 3)
	from := store.AddDays(t, -45-days)
	txs, err := conn.client.Transactions(ctx, "income", from, t, 300)
	if err != nil {
		return nil, "Pocket Money did not answer: " + err.Error(), nil
	}
	linked, _ := s.store.MoneyRefs(ctx, uid)
	margin := float64(max(st.MoneyMatchMargin, 0)) / 100
	var out []suggestion
	for _, tx := range txs {
		ref := strconv.FormatInt(tx.ID, 10)
		if linked[ref] || tx.Source == "rentals" || tx.Amount == 0 {
			continue
		}
		amt := tx.Amount
		if amt < 0 {
			amt = -amt
		}
		var best *suggestion
		for _, l := range b.leases {
			if l.Status != store.LeaseActive || (onlyLease > 0 && l.ID != onlyLease) || !strings.EqualFold(l.Currency, tx.Currency) {
				continue
			}
			// what this lease owes around that day: unpaid charges due near it
			var owed int64
			near := false
			for _, ch := range b.charges[l.ID] {
				if ch.Waived || ch.State == "paid" {
					continue
				}
				if d := daysBetween(ch.DueDate, tx.Date); d >= -days && d <= 45 {
					owed += ch.Amount - ch.Paid
					if d <= days {
						near = true
					}
				}
			}
			if owed <= 0 {
				continue
			}
			score := 0
			why := []string{}
			switch {
			case math.Abs(float64(amt-l.Rent)) <= float64(l.Rent)*margin:
				score += 3
				why = append(why, "the rent")
			case math.Abs(float64(amt-owed)) <= float64(owed)*margin:
				score += 3
				why = append(why, "what is owed")
			case amt < owed && amt > l.Rent/3:
				score++
				why = append(why, "part of what is owed")
			default:
				continue
			}
			if near {
				score++
				why = append(why, "near the due day")
			}
			if nameHit(tx.Payee+" "+tx.Description, l.Tenants) {
				score += 3
				why = append(why, "the tenant's name")
			}
			if score < 3 {
				continue
			}
			if best == nil || score > best.Score {
				best = &suggestion{TxID: ref, Date: tx.Date, Amount: amt, Currency: tx.Currency, Payee: tx.Payee, Description: tx.Description, Account: tx.AccountName,
					URL: strings.TrimRight(conn.public, "/") + "/transactions/" + ref, LeaseID: l.ID, LeaseTitle: l.Title, Owed: owed, Why: strings.Join(why, ", "), Score: score}
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Date > out[j].Date
	})
	return out, "", nil
}

func (s *Server) handleMoneySuggestions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if !s.settingsOf(ctx, uid).MoneyMatch && !queryBool(r, "force") {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "off": "Looking in Pocket Money is switched off (Settings → Rent)"})
		return
	}
	list, off, err := s.suggestions(ctx, uid, int64(queryInt(r, "lease")))
	if err != nil {
		s.fail(w, err, "money")
		return
	}
	if list == nil {
		list = []suggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "off": off})
}

// handleMoneyMatch says "this transaction is that lease's rent": a payment
// linked to it, nothing booked twice.
func (s *Server) handleMoneyMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		TxID    string `json:"tx_id"`
		LeaseID int64  `json:"lease_id"`
		Date    string `json:"date"`
		Amount  int64  `json:"amount"`
	}
	if err := decode(r, &in); err != nil || in.TxID == "" {
		writeErr(w, http.StatusBadRequest, "say which transaction")
		return
	}
	l, err := s.store.GetLease(ctx, uid, in.LeaseID)
	if err != nil {
		s.fail(w, err, "lease")
		return
	}
	// read it from Money, so the day and the amount are what the bank says
	c := s.clients(ctx, uid)
	if conn, err := c.conn(ctx, appMoney, scopeTxRead); err == nil {
		var tx pocket.MoneyTx
		if err := conn.client.Do(ctx, http.MethodGet, "/api/v1/transactions/"+in.TxID, nil, nil, &tx); err == nil && tx.ID > 0 {
			in.Date, in.Amount = tx.Date, tx.Amount
			if in.Amount < 0 {
				in.Amount = -in.Amount
			}
		}
	}
	if in.Amount <= 0 || !validDate(in.Date) || in.Date == "" {
		writeErr(w, http.StatusBadRequest, "Pocket Money did not say the day and amount of that transaction")
		return
	}
	ref, f := in.TxID, false
	p, out, err := s.addPayment(ctx, uid, l, paymentIn{Date: &in.Date, Amount: &in.Amount, MoneyTxRef: &ref, Money: &f, Note: ptr("Found in Pocket Money")}, "")
	if err != nil {
		s.userFail(w, err, "payment")
		return
	}
	out["payment"] = p
	writeJSON(w, http.StatusCreated, out)
}
