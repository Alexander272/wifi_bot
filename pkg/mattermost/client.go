package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(serverURL, botToken string) *Client {
	base := strings.TrimRight(serverURL, "/")
	return &Client{
		baseURL: base,
		token:   botToken,
		http:    &http.Client{},
	}
}

type Post struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	RootID    string `json:"root_id"`
	CreateAt  int64  `json:"create_at"`
}

type sendPostReq struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}

func (c *Client) SendPost(channelID, message string) error {
	body := sendPostReq{ChannelID: channelID, Message: message}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal post body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v4/posts", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send post: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("send post: unexpected status %d", resp.StatusCode)
	}
	return nil
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Roles    string `json:"roles"`
}

func (c *Client) GetUser(userID string) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v4/users/"+userID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user: status %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	return &user, nil
}

func (c *Client) GetActiveTeamUsers(teamID string) ([]User, error) {
	var all []User
	page := 0

	for {
		url := fmt.Sprintf("%s/api/v4/users?in_team=%s&not_inactive=true&page=%d&per_page=200",
			c.baseURL, teamID, page)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get users: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read users response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("get users: status %d body %s", resp.StatusCode, string(body))
		}

		var users []User
		if err := json.Unmarshal(body, &users); err != nil {
			return nil, fmt.Errorf("decode users: %w", err)
		}

		if len(users) == 0 {
			break
		}

		all = append(all, users...)
		page++
	}

	return all, nil
}

func (c *Client) GetMe() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v4/users/me", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get me: unexpected status %d", resp.StatusCode)
	}

	var u struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("decode user: %w", err)
	}
	return u.ID, nil
}

func (c *Client) GetTeamByName(name string) (*Team, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v4/teams/name/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get team by name: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get team by name: status %d body %s", resp.StatusCode, string(body))
	}

	var team Team
	if err := json.NewDecoder(resp.Body).Decode(&team); err != nil {
		return nil, fmt.Errorf("decode team: %w", err)
	}
	return &team, nil
}
