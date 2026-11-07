package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Kinds and states (the API checks against these).
var (
	PropertyKinds  = []string{"house", "apartment", "building", "room", "commercial", "garage", "other"}
	Frequencies    = []string{"monthly", "weekly", "quarterly", "yearly"}
	LeaseStates    = []string{"draft", "active", "ended"}
	DepositStates  = []string{"none", "due", "held", "returned", "part_returned"}
	ChargeKinds    = []string{"rent", "deposit", "utilities", "late_fee", "repair", "other"}
	PayMethods     = []string{"bank", "cash", "card", "other"}
	Priorities     = []string{"low", "normal", "urgent"}
	RequestStates  = []string{"new", "planned", "in_progress", "waiting", "done", "cancelled"}
	ExpenseKinds   = []string{"repairs", "insurance", "tax", "utilities", "loan_interest", "building_fee", "management", "cleaning", "furniture", "supplies", "legal", "other"}
	InspectionKind = []string{"move_in", "move_out", "routine"}
	Conditions     = []string{"", "good", "fair", "poor", "damaged", "missing"}
)

// Lease states.
const (
	LeaseDraft  = "draft"
	LeaseActive = "active"
	LeaseEnded  = "ended"
)

// Property is a place you rent out.
type Property struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Address   string `json:"address"`
	City      string `json:"city"`
	Country   string `json:"country"`
	Currency  string `json:"currency"`
	Value     int64  `json:"value"`
	BoughtOn  string `json:"bought_on"`
	BoughtFor int64  `json:"bought_for"`
	HomeRef   string `json:"home_ref"`
	MoneyTag  string `json:"money_tag"`
	Color     string `json:"color"`
	Notes     string `json:"notes"`
	Archived  bool   `json:"archived"`
	Position  int    `json:"position"`
	ClientKey string `json:"-"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Filled for lists and pages.
	Units      []*Unit `json:"units,omitempty"`
	UnitCount  int     `json:"unit_count"`
	Let        int     `json:"let"`         // units with an active lease
	RentRoll   int64   `json:"rent_roll"`   // monthly rent of the active leases (in the property's currency)
	OpenRepair int     `json:"open_repair"` // repairs not done
	Owed       int64   `json:"owed"`        // rent overdue now
}

// Unit is what is let on its own.
type Unit struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"-"`
	PropertyID int64   `json:"property_id"`
	Name       string  `json:"name"`
	Bedrooms   int     `json:"bedrooms"`
	Bathrooms  int     `json:"bathrooms"`
	AreaM2     float64 `json:"area_m2"`
	Rent       int64   `json:"rent"`
	Deposit    int64   `json:"deposit"`
	Notes      string  `json:"notes"`
	Position   int     `json:"position"`
	Archived   bool    `json:"archived"`
	CreatedAt  string  `json:"created_at"`

	// Filled for lists.
	PropertyName string `json:"property_name,omitempty"`
	Lease        *Lease `json:"lease,omitempty"` // the active (or next) lease
}

// Tenant is someone who rents from you.
type Tenant struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	ContactRef string `json:"contact_ref"`
	Notes      string `json:"notes"`
	ClientKey  string `json:"-"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`

	// Filled for lists.
	Leases  int    `json:"leases"`
	Current string `json:"current,omitempty"` // where they rent now
	LeaseID int64  `json:"lease_id,omitempty"`
	Balance int64  `json:"balance"`
}

// Lease is a unit let to tenants.
type Lease struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"-"`
	UnitID       int64  `json:"unit_id"`
	Status       string `json:"status"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	ChargesFrom  string `json:"charges_from"`
	Rent         int64  `json:"rent"`
	Currency     string `json:"currency"`
	Frequency    string `json:"frequency"`
	DueDay       int    `json:"due_day"`
	GraceDays    int    `json:"grace_days"`
	LateFee      int64  `json:"late_fee"`
	NoticeDays   int    `json:"notice_days"`
	Deposit      int64  `json:"deposit"`
	DepositState string `json:"deposit_state"`
	DepositBack  int64  `json:"deposit_back"`
	DepositNote  string `json:"deposit_note"`
	EndedOn      string `json:"ended_on"`
	EndReason    string `json:"end_reason"`
	SigningRef   string `json:"signing_ref"`
	SigningState string `json:"signing_state"`
	Notes        string `json:"notes"`
	SourceApp    string `json:"source_app"`
	ClientKey    string `json:"-"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`

	// Filled for lists and pages.
	UnitName     string    `json:"unit_name"`
	PropertyID   int64     `json:"property_id"`
	PropertyName string    `json:"property_name"`
	Tenants      []*Tenant `json:"tenants"`
	Balance      int64     `json:"balance"`  // owed now (charges due − payments); < 0 = paid ahead
	Overdue      int64     `json:"overdue"`  // owed past the grace days
	NextDue      string    `json:"next_due"` // the next rent day
	DaysLeft     *int      `json:"days_left,omitempty"`
	Title        string    `json:"title"`
}

// Charge is what a lease owes.
type Charge struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	LeaseID    int64  `json:"lease_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Period     string `json:"period"`
	DueDate    string `json:"due_date"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Waived     bool   `json:"waived"`
	InvoiceRef string `json:"invoice_ref"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`

	// Worked out from the payments (oldest charges are paid first).
	Paid   int64  `json:"paid"`
	State  string `json:"state"` // paid | part | due | late | upcoming | waived
	PaidOn string `json:"paid_on,omitempty"`
}

// Payment is money in from a lease.
type Payment struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	LeaseID    int64  `json:"lease_id"`
	Date       string `json:"date"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Method     string `json:"method"`
	Note       string `json:"note"`
	MoneyTxRef string `json:"money_tx_ref"`
	MoneyFound bool   `json:"money_found"`
	SourceApp  string `json:"source_app"`
	ClientKey  string `json:"-"`
	CreatedAt  string `json:"created_at"`

	MoneyURL string `json:"money_url,omitempty"`
}

// Request is a repair or something to fix.
type Request struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"-"`
	PropertyID    int64  `json:"property_id"`
	UnitID        *int64 `json:"unit_id"`
	LeaseID       *int64 `json:"lease_id"`
	Title         string `json:"title"`
	Details       string `json:"details"`
	Priority      string `json:"priority"`
	Status        string `json:"status"`
	ReportedBy    string `json:"reported_by"`
	ReportedOn    string `json:"reported_on"`
	PlannedOn     string `json:"planned_on"`
	PlannedTime   string `json:"planned_time"`
	DoneOn        string `json:"done_on"`
	Cost          int64  `json:"cost"`
	Currency      string `json:"currency"`
	ProName       string `json:"pro_name"`
	ProPhone      string `json:"pro_phone"`
	ProRef        string `json:"pro_ref"`
	TenantPays    bool   `json:"tenant_pays"`
	TaskRef       string `json:"task_ref"`
	EventRef      string `json:"event_ref"`
	HomeRecordRef string `json:"home_record_ref"`
	SourceApp     string `json:"source_app"`
	SourceRef     string `json:"source_ref"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`

	PropertyName string `json:"property_name"`
	UnitName     string `json:"unit_name"`
	Photos       int    `json:"photos"`
}

// Open reports whether a repair still needs doing.
func (r *Request) Open() bool { return r.Status != "done" && r.Status != "cancelled" }

// Expense is money out for a place.
type Expense struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	PropertyID int64  `json:"property_id"`
	UnitID     *int64 `json:"unit_id"`
	RequestID  *int64 `json:"request_id"`
	Date       string `json:"date"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Category   string `json:"category"`
	Payee      string `json:"payee"`
	Note       string `json:"note"`
	MoneyTxRef string `json:"money_tx_ref"`
	MoneyFound bool   `json:"money_found"`
	ClientKey  string `json:"-"`
	CreatedAt  string `json:"created_at"`

	PropertyName string `json:"property_name"`
	MoneyURL     string `json:"money_url,omitempty"`
}

// Inspection is a walk through a unit.
type Inspection struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	PropertyID int64  `json:"property_id"`
	UnitID     *int64 `json:"unit_id"`
	LeaseID    *int64 `json:"lease_id"`
	Kind       string `json:"kind"`
	Date       string `json:"date"`
	Time       string `json:"time"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	EventRef   string `json:"event_ref"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`

	PropertyName string            `json:"property_name"`
	UnitName     string            `json:"unit_name"`
	Items        []*InspectionItem `json:"items,omitempty"`
	ItemCount    int               `json:"item_count"`
	Issues       int               `json:"issues"` // poor, damaged or missing
}

// InspectionItem is one line of an inspection.
type InspectionItem struct {
	ID           int64   `json:"id"`
	UserID       int64   `json:"-"`
	InspectionID int64   `json:"inspection_id"`
	Room         string  `json:"room"`
	Item         string  `json:"item"`
	Condition    string  `json:"condition"`
	Note         string  `json:"note"`
	Position     int     `json:"position"`
	Photos       []*File `json:"photos"`
}

// File is a photo or a file kept by the app.
type File struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Owner     string `json:"owner"`
	OwnerID   int64  `json:"owner_id"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// Entry is one line of a timeline.
type Entry struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	LeaseID    *int64 `json:"lease_id"`
	PropertyID *int64 `json:"property_id"`
	RequestID  *int64 `json:"request_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	At         string `json:"at"`
	App        string `json:"app"`
	URL        string `json:"url"`
	SourceApp  string `json:"source_app"`
	CreatedAt  string `json:"created_at"`
}

// Made is something a thing here has in another Pocket app.
type Made struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Owner     string `json:"owner"`
	OwnerID   int64  `json:"owner_id"`
	Target    string `json:"target"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Rent.
	Currency   string `json:"currency"`    // new places and leases start in this currency
	DueDay     int    `json:"due_day"`     // new leases: rent is due on this day of the month
	GraceDays  int    `json:"grace_days"`  // new leases: days after the due day before rent counts as late
	NoticeDays int    `json:"notice_days"` // new leases: notice either side gives
	RemindDays int    `json:"remind_days"` // the morning note mentions rent due this many days ahead
	EndingDays int    `json:"ending_days"` // a lease "ends soon" this many days before its end

	// Pocket Money.
	MoneyBook        bool  `json:"money_book"`        // payments and expenses go to Pocket Money as you add them
	MoneyMatch       bool  `json:"money_match"`       // look in Pocket Money for rent that came in
	MoneyAccount     int64 `json:"money_account"`     // 0 = the everyday account
	MoneyIncomeCat   int64 `json:"money_income_cat"`  // category for rent (0 = none)
	MoneyExpenseCat  int64 `json:"money_expense_cat"` // category for expenses (0 = none)
	MoneyMatchDays   int   `json:"money_match_days"`  // how far from the due day a payment may be
	MoneyMatchMargin int   `json:"money_match_pct"`   // how far off the amount may be, in percent

	// The other apps.
	RepairTasks  bool   `json:"repair_tasks"`  // a new repair becomes a task in Pocket Tasks
	CalendarSync bool   `json:"calendar_sync"` // inspections, repair visits and lease ends go to Pocket Calendar
	HomeLog      bool   `json:"home_log"`      // finished repairs go into the place's log in Pocket Home
	MailFrom     string `json:"mail_from"`     // which Pocket Mail account emails leave from ("" = its default)
	RepairForm   string `json:"repair_form"`   // the Pocket Forms form tenants report repairs with ("" = any)
	ReminderText string `json:"reminder_text"` // the rent reminder, with {name} {amount} {due} {place}
	InvoiceRent  bool   `json:"invoice_rent"`  // offer a draft invoice for each rent
	AIHelp       bool   `json:"ai_help"`

	// Notifications.
	NotifyDigest bool   `json:"notify_digest"` // morning: rent due and late, leases ending, repairs
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// DefaultReminder is the rent reminder until a person writes their own.
const DefaultReminder = "Hello {name},\n\na friendly reminder that the rent for {place}, {amount}, is due on {due}.\n\nIf you have already paid, thank you, and please ignore this note.\n\nBest regards"

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
