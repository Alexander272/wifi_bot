package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo/memory"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type mockMikrotikClient struct {
	mu                 sync.Mutex
	disconnectCalls    []string
	addBindingCalls    []string
	removeBindingCalls []string
	blockBindingCalls  []string
	addBindingErr      error
}

func (m *mockMikrotikClient) Disconnect(_ context.Context, mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnectCalls = append(m.disconnectCalls, mac)
	return nil
}

func (m *mockMikrotikClient) AddBinding(_ context.Context, mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addBindingCalls = append(m.addBindingCalls, mac)
	return m.addBindingErr
}

func (m *mockMikrotikClient) RemoveBinding(_ context.Context, mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeBindingCalls = append(m.removeBindingCalls, mac)
	return nil
}

func (m *mockMikrotikClient) BlockBinding(_ context.Context, mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockBindingCalls = append(m.blockBindingCalls, mac)
	return nil
}

func (m *mockMikrotikClient) ListSessions(_ context.Context) ([]mikrotikClient.HotspotSession, error) {
	return nil, nil
}

func (m *mockMikrotikClient) AddAddressToList(_ context.Context, _, _, _ string) error { return nil }

func (m *mockMikrotikClient) RemoveAddressFromList(_ context.Context, _, _ string) error { return nil }

func (m *mockMikrotikClient) ListAddressList(_ context.Context, _ string) ([]mikrotikClient.AddressListEntry, error) { return nil, nil }

func (m *mockMikrotikClient) DisconnectCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]string, len(m.disconnectCalls))
	copy(r, m.disconnectCalls)
	return r
}

func (m *mockMikrotikClient) RemoveBindingCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]string, len(m.removeBindingCalls))
	copy(r, m.removeBindingCalls)
	return r
}

func (m *mockMikrotikClient) AddBindingCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]string, len(m.addBindingCalls))
	copy(r, m.addBindingCalls)
	return r
}

type sessionTestFixture struct {
	svc             *SessionService
	sessionRepo     *memory.SessionRepo
	logRepo         *memory.LogRepo
	userSessionRepo *memory.UserSessionRepo
	client          *mockMikrotikClient
	code            *CodeService
}

func newSessionTestFixture(t *testing.T, allowReuse bool, authMethod string) *sessionTestFixture {
	t.Helper()
	sessionRepo := memory.NewSessionRepo(time.Hour)
	logRepo := memory.NewLogRepo()
	userSessionRepo := memory.NewUserSessionRepo()
	codeService := NewCodeService()

	client := &mockMikrotikClient{}

	mikrotikSvc := NewMikrotikService(client, time.Second, "192.168.1.1", authMethod, "")

	svc := NewSessionService(&SessionDeps{
		SessionRepo:     sessionRepo,
		LogRepo:         logRepo,
		UserSessionRepo: userSessionRepo,
		Code:            codeService,
		Mikrotik:        mikrotikSvc,
		AllowReuse:      allowReuse,
	})

	return &sessionTestFixture{
		svc:             svc,
		sessionRepo:     sessionRepo,
		logRepo:         logRepo,
		userSessionRepo: userSessionRepo,
		client:          client,
		code:            codeService,
	}
}

func TestGetOrCreateCode(t *testing.T) {
	t.Parallel()

	t.Run("new user creates code", func(t *testing.T) {
		t.Parallel()
		f := newSessionTestFixture(t, false, "chap")
		code, err := f.svc.GetOrCreateCode(context.Background(), "user1", "")
		require.NoError(t, err)
		assert.Len(t, code, 6)
		assert.True(t, f.code.IsValid(code))
	})

	t.Run("existing user returns same code", func(t *testing.T) {
		t.Parallel()
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		code1, err := f.svc.GetOrCreateCode(ctx, "user1", "")
		require.NoError(t, err)

		code2, err := f.svc.GetOrCreateCode(ctx, "user1", "")
		require.NoError(t, err)
		assert.Equal(t, code1, code2)
	})

	t.Run("different users get different codes", func(t *testing.T) {
		t.Parallel()
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		code1, _ := f.svc.GetOrCreateCode(ctx, "user_a", "")
		code2, _ := f.svc.GetOrCreateCode(ctx, "user_b", "")
		assert.NotEqual(t, code1, code2)
	})
}

func TestResetCode(t *testing.T) {
	t.Parallel()

	t.Run("with MAC in Redis disconnects and creates new code", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		code, err := f.svc.GetOrCreateCode(ctx, "user1", "")
		require.NoError(t, err)

		err = f.sessionRepo.UpdateMac(ctx, code, "aa:bb:cc:dd:ee:01", "10.0.0.1")
		require.NoError(t, err)
		err = f.userSessionRepo.Create(ctx, &models.UserSession{
			UserID: "user1", Code: code, Mac: "aa:bb:cc:dd:ee:01", IP: "10.0.0.1",
			LoginAt: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		newCode, err := f.svc.ResetCode(ctx, "user1", "")
		require.NoError(t, err)
		assert.NotEqual(t, code, newCode)
		assert.Len(t, newCode, 6)

		time.Sleep(50 * time.Millisecond)

		assert.Contains(t, f.client.RemoveBindingCalls(), "aa:bb:cc:dd:ee:01")
		assert.Contains(t, f.client.DisconnectCalls(), "aa:bb:cc:dd:ee:01")

		active, err := f.userSessionRepo.ListActive(ctx)
		require.NoError(t, err)
		assert.Empty(t, active)

		session, err := f.sessionRepo.GetByUser(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, newCode, session.Code)
	})

	t.Run("with MAC only in user_sessions disconnects from DB fallback", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		err := f.userSessionRepo.Create(ctx, &models.UserSession{
			UserID: "user1", Code: "XXXXXX", Mac: "aa:bb:cc:dd:ee:02", IP: "10.0.0.2",
			LoginAt: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		newCode, err := f.svc.ResetCode(ctx, "user1", "")
		require.NoError(t, err)
		assert.Len(t, newCode, 6)

		time.Sleep(50 * time.Millisecond)

		assert.Contains(t, f.client.RemoveBindingCalls(), "aa:bb:cc:dd:ee:02")
		assert.Contains(t, f.client.DisconnectCalls(), "aa:bb:cc:dd:ee:02")

		active, err := f.userSessionRepo.ListActive(ctx)
		require.NoError(t, err)
		assert.Empty(t, active)
	})

	t.Run("without MAC only resets code", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		oldCode, err := f.svc.GetOrCreateCode(ctx, "user1", "")
		require.NoError(t, err)

		newCode, err := f.svc.ResetCode(ctx, "user1", "")
		require.NoError(t, err)
		assert.NotEqual(t, oldCode, newCode)

		time.Sleep(50 * time.Millisecond)

		assert.Empty(t, f.client.RemoveBindingCalls())
		assert.Empty(t, f.client.DisconnectCalls())

		session, err := f.sessionRepo.GetByUser(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, newCode, session.Code)
	})

	t.Run("without any session still creates code", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		code, err := f.svc.ResetCode(ctx, "unknown_user", "")
		require.NoError(t, err)
		assert.Len(t, code, 6)

		time.Sleep(50 * time.Millisecond)
		assert.Empty(t, f.client.DisconnectCalls())
	})

	t.Run("with existing user_session closes it after reset", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.userSessionRepo.Create(ctx, &models.UserSession{
			UserID: "user1", Code: "OLDCOD", Mac: "aa:bb:cc:dd:ee:03", IP: "10.0.0.3",
			LoginAt: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		_, err = f.svc.ResetCode(ctx, "user1", "")
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		active, err := f.userSessionRepo.ListActive(ctx)
		require.NoError(t, err)
		assert.Empty(t, active)

		assert.Contains(t, f.client.DisconnectCalls(), "aa:bb:cc:dd:ee:03")
	})
}

func TestLogin(t *testing.T) {
	t.Parallel()

	t.Run("invalid code format returns error", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.svc.Login(ctx, "short", "aa:bb:cc:dd:ee:10", "10.0.0.10", "", "", "")
		assert.ErrorIs(t, err, models.ErrCodeInvalid)

		err = f.svc.Login(ctx, "123456", "aa:bb:cc:dd:ee:10", "10.0.0.10", "", "", "")
		assert.ErrorIs(t, err, models.ErrCodeInvalid)
	})

	t.Run("code not found returns error", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.svc.Login(ctx, "ABCDEF", "aa:bb:cc:dd:ee:11", "10.0.0.11", "", "", "")
		assert.Error(t, err)
		assert.ErrorContains(t, err, "invalid code")
	})

	t.Run("first login succeeds and creates user_session", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		code, err := f.svc.GetOrCreateCode(ctx, "user1", "")
		require.NoError(t, err)

		err = f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:12", "10.0.0.12", "", "", "")
		require.NoError(t, err)

		assert.Contains(t, f.client.AddBindingCalls(), "aa:bb:cc:dd:ee:12")

		us, err := f.userSessionRepo.GetActiveByMAC(ctx, "aa:bb:cc:dd:ee:12")
		require.NoError(t, err)
		require.NotNil(t, us)
		assert.Equal(t, "user1", us.UserID)
		assert.True(t, us.IsActive)

		s, err := f.sessionRepo.GetByUser(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:12", s.Mac)
	})

	t.Run("same MAC relogin succeeds", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		code, _ := f.svc.GetOrCreateCode(ctx, "user1", "")

		err := f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:13", "10.0.0.13", "", "", "")
		require.NoError(t, err)

		err = f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:13", "10.0.0.13", "", "", "")
		require.NoError(t, err)
	})

	t.Run("different MAC with allowReuse=false returns ErrCodeAlreadyUsed", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		code, _ := f.svc.GetOrCreateCode(ctx, "user1", "")

		err := f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:14", "10.0.0.14", "", "", "")
		require.NoError(t, err)

		err = f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:15", "10.0.0.15", "", "", "")
		assert.ErrorIs(t, err, models.ErrCodeAlreadyUsed)
	})

	t.Run("different MAC with allowReuse=true disconnects old and proceeds", func(t *testing.T) {
		f := newSessionTestFixture(t, true, "mac_binding")
		ctx := context.Background()

		code, _ := f.svc.GetOrCreateCode(ctx, "user1", "")

		err := f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:16", "10.0.0.16", "", "", "")
		require.NoError(t, err)

		err = f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:17", "10.0.0.17", "", "", "")
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		assert.Contains(t, f.client.DisconnectCalls(), "aa:bb:cc:dd:ee:16")

		s, err := f.sessionRepo.GetByUser(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:17", s.Mac)

		newSession, err := f.userSessionRepo.GetActiveByMAC(ctx, "aa:bb:cc:dd:ee:17")
		require.NoError(t, err)
		require.NotNil(t, newSession)
		assert.True(t, newSession.IsActive)

		oldSession, err := f.userSessionRepo.GetActiveByMAC(ctx, "aa:bb:cc:dd:ee:16")
		require.NoError(t, err)
		assert.Nil(t, oldSession)
	})

	t.Run("auth failure does not create user_session", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "mac_binding")
		ctx := context.Background()

		f.client.addBindingErr = assert.AnError

		code, _ := f.svc.GetOrCreateCode(ctx, "user1", "")

		err := f.svc.Login(ctx, code, "aa:bb:cc:dd:ee:18", "10.0.0.18", "", "", "")
		assert.ErrorIs(t, err, models.ErrMikrotikAuth)

		us, err := f.userSessionRepo.GetActiveByMAC(ctx, "aa:bb:cc:dd:ee:18")
		require.NoError(t, err)
		assert.Nil(t, us)
	})
}

func TestResolveMAC(t *testing.T) {
	t.Parallel()

	t.Run("returns MAC from Redis when present", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.sessionRepo.Create(ctx, &models.WifiSession{
			UserID: "user1", Code: "ABCDEF", Mac: "aa:bb:cc:dd:ee:20", CreatedAt: time.Now(),
		})
		require.NoError(t, err)
		err = f.sessionRepo.UpdateMac(ctx, "ABCDEF", "aa:bb:cc:dd:ee:20", "10.0.0.20")
		require.NoError(t, err)

		mac, err := f.svc.resolveMAC(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:20", mac)
	})

	t.Run("falls back to user_sessions when Redis is empty", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.userSessionRepo.Create(ctx, &models.UserSession{
			UserID: "user1", Code: "FEDCBA", Mac: "aa:bb:cc:dd:ee:21", IP: "10.0.0.21",
			LoginAt: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		mac, err := f.svc.resolveMAC(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:21", mac)
	})

	t.Run("prefers Redis over user_sessions", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		err := f.sessionRepo.Create(ctx, &models.WifiSession{
			UserID: "user1", Code: "ABCDEF", Mac: "aa:bb:cc:dd:ee:22", CreatedAt: time.Now(),
		})
		require.NoError(t, err)
		err = f.sessionRepo.UpdateMac(ctx, "ABCDEF", "aa:bb:cc:dd:ee:22", "10.0.0.22")
		require.NoError(t, err)

		err = f.userSessionRepo.Create(ctx, &models.UserSession{
			UserID: "user1", Code: "ZZZZZZ", Mac: "aa:bb:cc:dd:ee:99", IP: "10.0.0.99",
			LoginAt: time.Now(), IsActive: true,
		})
		require.NoError(t, err)

		mac, err := f.svc.resolveMAC(ctx, "user1")
		require.NoError(t, err)
		assert.Equal(t, "aa:bb:cc:dd:ee:22", mac)
	})

	t.Run("returns empty when both are empty", func(t *testing.T) {
		f := newSessionTestFixture(t, false, "chap")
		ctx := context.Background()

		mac, err := f.svc.resolveMAC(ctx, "unknown_user")
		require.NoError(t, err)
		assert.Empty(t, mac)
	})
}
