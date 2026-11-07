// Package pocket talks to sibling Pocket apps for League: Pocket Calendar
// (the games you are in), Pocket Health (the matches you played), Pocket
// Habits, Pocket Weather (the forecast for a game), Pocket Split and Pocket
// Money (what a game costs), Pocket Places (the venue), Pocket Contacts (the
// players), Pocket Mail (invitations) and Pocket Tasks (book the court).
// Same bearer-token JSON API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// ---------------- Pocket Money ----------------

// Account is a Money account the person can see.
type Account struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Currency string `json:"currency"`
	Archived bool   `json:"archived"`
	Role     string `json:"role"` // owner | editor | viewer
}

// Accounts lists the accounts the person can book on.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var all []Account
	if err := c.do(ctx, http.MethodGet, "/api/v1/accounts", nil, nil, &all); err != nil {
		return nil, err
	}
	out := []Account{}
	for _, a := range all {
		if !a.Archived && a.Role != "viewer" {
			out = append(out, a)
		}
	}
	return out, nil
}

// Category is a Money category.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Kind     string `json:"kind"`
	Archived bool   `json:"archived"`
}

// Categories lists the expense categories usable on an account.
func (c *Client) Categories(ctx context.Context, accountID int64) ([]Category, error) {
	var all []Category
	path := "/api/v1/categories"
	if accountID > 0 {
		path = fmt.Sprintf("/api/v1/accounts/%d/categories", accountID)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &all); err != nil {
		return nil, err
	}
	out := []Category{}
	for _, ct := range all {
		if !ct.Archived && ct.Kind != "income" {
			out = append(out, ct)
		}
	}
	return out, nil
}

// Tx is what League books: always an expense (a person's share of a game).
type Tx struct {
	AccountID   int64
	Date        string
	Time        string
	Amount      int64 // minor units of Currency
	Currency    string
	Description string
	Payee       string
	CategoryID  int64
	Notes       string
	ExternalID  string
}

// CreateTx books a new expense and returns its id.
func (c *Client) CreateTx(ctx context.Context, t Tx) (int64, error) {
	body := map[string]any{"kind": "expense", "account_id": t.AccountID, "date": t.Date, "amount": t.Amount, "currency": t.Currency,
		"description": t.Description, "payee": t.Payee, "tags": []string{"sport"}, "notes": t.Notes, "source": "league", "external_id": t.ExternalID}
	if t.CategoryID > 0 {
		body["category_id"] = t.CategoryID
	}
	if t.Time != "" {
		body["time"] = t.Time
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/transactions", nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// DeleteTx removes a transaction; one already gone counts as deleted.
func (c *Client) DeleteTx(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/transactions/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ---------------- Pocket Calendar ----------------

// Calendar is one of the person's calendars.
type Calendar struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Writable bool   `json:"writable"`
	Default  bool   `json:"is_default"`
}

// Calendars lists the calendars the person can write to.
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/calendars", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[Calendar](raw, "calendars")
	if err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, cal := range all {
		if cal.Writable {
			out = append(out, cal)
		}
	}
	return out, nil
}

// Event is a game on the calendar.
type Event struct {
	CalendarID  int64
	Title       string
	Description string
	Location    string
	URL         string
	Start       string // YYYY-MM-DDTHH:MM (local)
	Minutes     int
}

func (e Event) body() map[string]any {
	b := map[string]any{"title": e.Title, "description": e.Description, "location": e.Location, "url": e.URL, "categories": []string{"Sport"}}
	start, err := time.Parse("2006-01-02T15:04", e.Start)
	if err != nil {
		b["all_day"], b["start"] = true, e.Start
	} else {
		m := e.Minutes
		if m <= 0 {
			m = 90
		}
		b["all_day"], b["start"], b["end"] = false, start.Format("2006-01-02T15:04"), start.Add(time.Duration(m)*time.Minute).Format("2006-01-02T15:04")
	}
	if e.CalendarID > 0 {
		b["calendar_id"] = e.CalendarID
	}
	return b
}

// CreateEvent adds the event and returns its id.
func (c *Client) CreateEvent(ctx context.Context, e Event) (int64, error) {
	var out struct {
		EventID int64 `json:"event_id"`
		ID      any   `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/events", nil, e.body(), &out); err != nil {
		return 0, err
	}
	if out.EventID > 0 {
		return out.EventID, nil
	}
	// a bare {"id": 12} or an occurrence id like "12:20261104"
	var id int64
	_, _ = fmt.Sscan(strings.SplitN(fmt.Sprint(out.ID), ":", 2)[0], &id)
	return id, nil
}

// UpdateEvent moves or renames the event.
func (c *Client) UpdateEvent(ctx context.Context, id int64, e Event) error {
	b := e.body()
	delete(b, "calendar_id")
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/events/%d", id), nil, b, nil)
}

// DeleteEvent removes the event; one already gone counts as deleted.
func (c *Client) DeleteEvent(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/events/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Busy is something already in the person's calendar.
type Busy struct {
	Title  string `json:"title"`
	Start  string `json:"start"`
	End    string `json:"end"`
	AllDay bool   `json:"all_day"`
	Busy   *bool  `json:"busy"`
	Source string `json:"source"`
}

// Events lists what is in the person's calendars between two days (YYYY-MM-DD).
func (c *Client) Events(ctx context.Context, from, to string) ([]Busy, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/events", url.Values{"from": {from}, "to": {to}}, nil, &raw); err != nil {
		return nil, err
	}
	return list[Busy](raw, "events")
}

// ---------------- Pocket Contacts ----------------

// Contact is a card in Pocket Contacts.
type Contact struct {
	ID     int64    `json:"id"`
	FN     string   `json:"fn"`
	Org    string   `json:"org"`
	Emails []string `json:"emails"`
	Phones []string `json:"phones"`
}

// SearchContacts finds the person's contacts.
func (c *Client) SearchContacts(ctx context.Context, q string, limit int) ([]Contact, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/search", url.Values{"q": {q}, "limit": {fmt.Sprint(limit)}}, nil, &raw); err != nil {
		return nil, err
	}
	out, err := list[Contact](raw, "contacts")
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

// ---------------- Pocket Split ----------------

// SplitMember is someone in a Split group.
type SplitMember struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	UserID *int64 `json:"user_id"`
	Email  string `json:"email"`
}

// SplitGroup is a Pocket Split group the person is in.
type SplitGroup struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	Currency   string        `json:"currency"`
	Archived   bool          `json:"archived"`
	Members    []SplitMember `json:"members"`
	MyMemberID int64         `json:"my_member_id"`
}

// SplitGroups lists the person's open groups.
func (c *Client) SplitGroups(ctx context.Context) ([]SplitGroup, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/groups", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[SplitGroup](raw, "groups")
	if err != nil {
		return nil, err
	}
	out := []SplitGroup{}
	for _, g := range all {
		if !g.Archived && g.MyMemberID > 0 {
			out = append(out, g)
		}
	}
	return out, nil
}

// SplitExpense is a bill paid by one member, shared evenly by members.
type SplitExpense struct {
	GroupID int64
	Title   string
	Amount  int64
	Curr    string
	Date    string
	Notes   string
	PaidBy  int64
	Members []int64
}

// CreateSplitExpense adds the bill to the group and returns its id.
func (c *Client) CreateSplitExpense(ctx context.Context, e SplitExpense) (int64, error) {
	shares := make([]map[string]any, 0, len(e.Members))
	for _, m := range e.Members {
		shares = append(shares, map[string]any{"member_id": m, "share": 1})
	}
	body := map[string]any{"title": e.Title, "amount": e.Amount, "currency": e.Curr, "paid_by": e.PaidBy, "date": e.Date, "notes": e.Notes,
		"split_mode": "even", "shares": shares}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/groups/%d/expenses", e.GroupID), nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// DeleteSplitExpense removes a bill League added; one already gone counts as deleted.
func (c *Client) DeleteSplitExpense(ctx context.Context, group, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/groups/%d/expenses/%d", group, id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ---------------- Pocket Health ----------------

// NewHealthWorkout is a match sent to Pocket Health. Health keeps one per
// start time, kind and app, so sending it again updates it.
type NewHealthWorkout struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	StartAt   string `json:"start_at"`
	EndAt     string `json:"end_at"`
	DurationS int    `json:"duration_s"`
	Notes     string `json:"notes"`
}

// SendWorkout records a workout in Pocket Health and returns its id there.
func (c *Client) SendWorkout(ctx context.Context, w NewHealthWorkout) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/workouts", nil, w, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ---------------- Pocket Habits ----------------

// Habit is a Pocket Habits habit.
type Habit struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Habits lists the person's habits.
func (c *Client) Habits(ctx context.Context) ([]Habit, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/habits", nil, nil, &raw); err != nil {
		return nil, err
	}
	return list[Habit](raw, "habits")
}

// MarkHabit ticks a habit on a date (status 1 = done).
func (c *Client) MarkHabit(ctx context.Context, habitID int64, date string) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/habits/%d/entries/%s", habitID, date), nil, map[string]any{"status": 1}, nil)
}

// ---------------- Pocket Weather ----------------

// WeatherHour is one hour of a forecast.
type WeatherHour struct {
	Time       string   `json:"time"`
	Temp       *float64 `json:"temp"`
	Feels      *float64 `json:"feels"`
	PrecipProb *float64 `json:"precip_prob"`
	Precip     *float64 `json:"precip"`
	Code       int      `json:"code"`
	Wind       *float64 `json:"wind"`
	Gust       *float64 `json:"gust"`
	Condition  string   `json:"condition"`
}

// WeatherDay is one day of a forecast.
type WeatherDay struct {
	Date       string   `json:"date"`
	Code       int      `json:"code"`
	Condition  string   `json:"condition"`
	Text       string   `json:"text"`
	TMax       *float64 `json:"t_max"`
	TMin       *float64 `json:"t_min"`
	PrecipProb *float64 `json:"precip_prob"`
}

// Forecast is the next days at a point (or at the person's home without one).
type Forecast struct {
	Location *struct {
		Name string `json:"name"`
	} `json:"location"`
	Hours []WeatherHour `json:"hours"`
	Days  []WeatherDay  `json:"days"`
}

// ForecastAt reads the forecast for a point (nil = the person's home place).
func (c *Client) ForecastAt(ctx context.Context, lat, lon *float64) (*Forecast, error) {
	q := url.Values{"hours": {"240"}}
	if lat != nil && lon != nil {
		q.Set("lat", fmt.Sprint(*lat))
		q.Set("lon", fmt.Sprint(*lon))
	}
	var out Forecast
	if err := c.do(ctx, http.MethodGet, "/api/v1/forecast", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------- Pocket Places ----------------

// Place is a place in Pocket Places.
type Place struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Address string   `json:"address"`
	City    string   `json:"city"`
	Lat     *float64 `json:"lat"`
	Lng     *float64 `json:"lng"`
}

// SearchPlaces finds the person's places by name or address.
func (c *Client) SearchPlaces(ctx context.Context, q string, limit int) ([]Place, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/places", url.Values{"q": {q}, "limit": {fmt.Sprint(limit)}}, nil, &raw); err != nil {
		return nil, err
	}
	out, err := list[Place](raw, "places", "items")
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

// SavePlace adds a venue to the person's places and returns its id.
func (c *Client) SavePlace(ctx context.Context, name, address string, lat, lon *float64, notes string) (int64, error) {
	body := map[string]any{"name": name, "kind": "activity", "status": "been", "address": address, "notes": notes, "tags": "sport"}
	if lat != nil && lon != nil {
		body["lat"], body["lng"] = *lat, *lon
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/places", nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ---------------- Pocket Mail ----------------

// MailAddress is a name and address as Pocket Mail takes them.
type MailAddress struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// Mail is one email Pocket Mail sends from one of the person's accounts.
type Mail struct {
	From     string        `json:"from,omitempty"` // which account ("" = Mail's default)
	To       []MailAddress `json:"to"`
	Subject  string        `json:"subject"`
	BodyHTML string        `json:"body_html"`
	BodyText string        `json:"body_text"`
}

// SendMail hands an email to Pocket Mail (it lands in the Sent folder).
func (c *Client) SendMail(ctx context.Context, m Mail) error {
	var out struct {
		Status string `json:"status"`
	}
	return c.do(ctx, http.MethodPost, "/api/v1/send", nil, m, &out)
}

// ---------------- Pocket Tasks ----------------

// Task is a task in Pocket Tasks.
type Task struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// CreateTask adds a task with a link back to League; due is YYYY-MM-DD or "".
func (c *Client) CreateTask(ctx context.Context, title, notes, due, link string) (*Task, error) {
	body := map[string]any{"title": title, "notes": notes, "url": link}
	if due != "" {
		body["due"] = due
	}
	var out Task
	if err := c.do(ctx, http.MethodPost, "/api/v1/tasks", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
