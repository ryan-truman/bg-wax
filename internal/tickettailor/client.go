package tickettailor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.tickettailor.com/v1"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type Event struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type IssuedTicket struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

func (t IssuedTicket) FullName() string {
	return t.FirstName + " " + t.LastName
}

// ListEvents returns every event on the account.
func (c *Client) ListEvents() ([]Event, error) {
	type response struct {
		Data []Event `json:"data"`
	}

	var result response
	if err := c.get("/events", nil, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetEvent fetches a single event by its ID.
func (c *Client) GetEvent(id string) (*Event, error) {
	var e Event
	if err := c.get("/events/"+url.PathEscape(id), nil, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// FindEventByName returns the first event whose name matches exactly.
func (c *Client) FindEventByName(name string) (*Event, error) {
	events, err := c.ListEvents()
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("no event found with name %q", name)
}

// TicketsForEvent fetches all issued tickets for an event, handling pagination.
func (c *Client) TicketsForEvent(eventID string) ([]IssuedTicket, error) {
	type response struct {
		Data []IssuedTicket `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}

	var all []IssuedTicket
	params := url.Values{"event_id": {eventID}, "limit": {"100"}}

	for {
		var page response
		if err := c.get("/issued_tickets", params, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)

		if page.Links.Next == "" {
			break
		}
		params.Set("starting_after", page.Data[len(page.Data)-1].ID)
	}

	return all, nil
}

func (c *Client) get(path string, params url.Values, out any) error {
	u := baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
