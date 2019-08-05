package analytics

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

const (
	testAccountID = "sakari-account-id"
	testApiKey    = "test-api-key"
)

func TestNew(t *testing.T) {
	type args struct {
		key     string
		account string
		opts    []Options
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "#1: Create with no user sourcing",
			args: args{
				key:     "Api-KEY",
				account: "Sakari-Account",
				opts:    []Options{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.args.key, tt.args.account, tt.args.opts...)

			assert.Equal(t, tt.args.key, got.key)
			assert.Equal(t, tt.args.account, got.skAccount)
			assert.NotNil(t, got.httpClient)
		})
	}
}

// User mock up sourcing
type UserSession struct {
	testUserId string
}

func (u *UserSession) GetUserId() string {
	// return your current user id
	return u.testUserId
}

func TestNew_With_UserSourcing(t *testing.T) {
	type args struct {
		key     string
		account string
		opts    []Options
	}
	tests := []struct {
		name           string
		args           args
		expectedUserID string
	}{
		{
			name: "#1: Create with user sourcing",
			args: args{
				key:     "Api-KEY",
				account: "Sakari-Account",
				opts: []Options{WithInitialUserSourcing(func() string {
					u := UserSession{testUserId: "test-1"}
					return u.GetUserId()
				})},
			},
			expectedUserID: "test-1",
		},
		{
			name: "#2: Create with anonyous user sourcing",
			args: args{
				key:     "Api-KEY",
				account: "Sakari-Account",
				opts: []Options{WithInitialUserSourcing(func() string {
					u := UserSession{testUserId: "anonymous-1"}
					return u.GetUserId()
				})},
			},
			expectedUserID: "anonymous-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.args.key, tt.args.account, tt.args.opts...)

			assert.Equal(t, got.user(), tt.expectedUserID)
		})
	}
}

func TestClient_Alias(t *testing.T) {
	type args struct {
		msg *Alias
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "#1: Empty userId",
			args: args{
				msg: &Alias{},
			},
			wantErr: errors.New("you must pass a 'alias.userId'"),
		},
		{
			name: "#2: Empty previousId",
			args: args{
				msg: &Alias{UserId: "user-id"},
			},
			wantErr: errors.New("you must pass a 'alias.previousId'"),
		},
		{
			name: "#3: Valid message",
			args: args{
				msg: &Alias{UserId: "user-id", PreviousId: "valid-previous"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenError := make(chan error, 10)
			createMockServer(Endpoint)
			defer gock.Off()
			c := createTestClientEnvironment(listenError)
			if err := c.Alias(tt.args.msg); !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("Client.Alias() error = %v, wantErr %v", err, tt.wantErr)
			}
			c.Close()
			if tt.wantErr == nil && len(listenError) > 0 {
				err := <-listenError
				t.Fatal(err)
			}
			if tt.wantErr == nil {
				assert.Equal(t, "alias", tt.args.msg.Type, "Type must be alias")
			}
			close(listenError)
		})
	}
}

func TestClient_Identify(t *testing.T) {
	type args struct {
		msg *Identify
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "#1: Empty userId",
			args: args{
				msg: &Identify{},
			},
			wantErr: errors.New("you must pass 'identify.userId'"),
		},
		{
			name: "#2: Valid message",
			args: args{
				msg: &Identify{UserId: "paul@sakari.ai"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenError := make(chan error, 10)
			createMockServer(Endpoint)
			defer gock.Off()
			c := createTestClientEnvironment(listenError)
			if err := c.Identify(tt.args.msg); !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("Client.Alias() error = %v, wantErr %v", err, tt.wantErr)
			}
			c.Close()
			if tt.wantErr == nil && len(listenError) > 0 {
				err := <-listenError
				t.Fatal(err)
			}
			if tt.wantErr == nil {
				assert.Equal(t, "identify", tt.args.msg.Type, "Type must be identify")
				assert.Equal(t, tt.args.msg.UserId, c.user(), "Hence we will use new user id for another message")
			}
			close(listenError)
		})
	}
}

func TestClient_Track(t *testing.T) {
	type args struct {
		msg *Track
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "#1: Empty event",
			args: args{
				msg: &Track{},
			},
			wantErr: errors.New("you must pass 'track.event'"),
		},
		{
			name: "#2: Valid message",
			args: args{
				msg: &Track{Event: "paul_sings"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenError := make(chan error, 10)
			createMockServer(Endpoint)
			defer gock.Off()
			c := createTestClientEnvironment(listenError)
			c.user = func() string {
				return "testing-user"
			}
			if err := c.Track(tt.args.msg); !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("Client.Alias() error = %v, wantErr %v", err, tt.wantErr)
			}
			c.Close()
			if tt.wantErr == nil && len(listenError) > 0 {
				err := <-listenError
				t.Fatal(err)
			}
			if tt.wantErr == nil {
				assert.Equal(t, "track", tt.args.msg.Type, "Type must be track")
				assert.Equal(t, "testing-user", tt.args.msg.UserId, "following user id from user sourcing")
			}
			close(listenError)
		})
	}
}

func TestClient_Page(t *testing.T) {
	type args struct {
		msg *Page
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "#1: Full Page event",
			args: args{
				msg: &Page{Name: "Paul page"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenError := make(chan error, 10)
			createMockServer(Endpoint)
			defer gock.Off()
			c := createTestClientEnvironment(listenError)
			c.user = func() string {
				return "testing-user"
			}
			if err := c.Page(tt.args.msg); !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("Client.Alias() error = %v, wantErr %v", err, tt.wantErr)
			}
			c.Close()
			if tt.wantErr == nil && len(listenError) > 0 {
				err := <-listenError
				t.Fatal(err)
			}
			if tt.wantErr == nil {
				assert.Equal(t, "page", tt.args.msg.Type, "Type must be page")
				assert.Equal(t, "testing-user", tt.args.msg.UserId, "following user id from user sourcing")
			}
			close(listenError)
		})
	}
}

func TestClient_Group(t *testing.T) {
	type args struct {
		msg *Group
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "#1: Empty group event",
			args: args{
				msg: &Group{},
			},
			wantErr: errors.New("you must pass a 'groupId'"),
		},
		{
			name: "#1: Full Group event",
			args: args{
				msg: &Group{GroupId: "new group user"},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenError := make(chan error, 10)
			createMockServer(Endpoint)
			defer gock.Off()
			c := createTestClientEnvironment(listenError)
			c.user = func() string {
				return "testing-user"
			}
			if err := c.Group(tt.args.msg); !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("Client.Alias() error = %v, wantErr %v", err, tt.wantErr)
			}
			c.Close()
			if tt.wantErr == nil && len(listenError) > 0 {
				err := <-listenError
				t.Fatal(err)
			}
			if tt.wantErr == nil {
				assert.Equal(t, "group", tt.args.msg.Type, "Type must be group")
				assert.Equal(t, "testing-user", tt.args.msg.UserId, "following user id from user sourcing")
			}
			close(listenError)
		})
	}
}

func createTestClientEnvironment(listen chan error) *Client {
	c := New(testApiKey, testAccountID)
	c.sleep = func(i int, err error) {
		listen <- err
	}
	c.retry = 1
	return c
}
func createMockServer(host string) {
	gock.New(host).Post("/v1/batch").
		MatchHeader("X-AccountID", testAccountID).
		MatchHeader("X-AuthSakari", testApiKey).
		Reply(200).
		Type("application/json")
}

func TestNewProperties(t *testing.T) {
	tests := []struct {
		name string
		want Properties
	}{
		{
			name: "#1: Create and set params",
			want: Properties{"paul": "Ann", "duration": 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewProperties()
			got.Set("paul", "Ann")
			got.Set("duration", 10)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestNewTraits(t *testing.T) {
	tests := []struct {
		name string
		want Trait
	}{
		{
			name: "#1: Create and set params",
			want: Trait{"paul": "Ann", "duration": 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewTrait()
			got.Set("paul", "Ann")
			got.Set("duration", 10)
			assert.Equal(t, got, tt.want)
		})
	}
}
