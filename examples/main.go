package main

import (
	"context"
	"github.com/sakari-ai/analytics-go"
)

// User mock up sourcing
// Ideally, this UserSession service must return user ID via Context
type UserSession struct {
}

//GetUserId will rely user-id in context
func (u *UserSession) GetUserId(ctx context.Context) string {
	return "user-id"
}

func main() {
	session := new(UserSession)
	sakari := analytics.New("uyKDw5O0p0t6IwuWAMtiMixT2vXpoLUH",
		"156c5bcd-2b8b-437c-b58c-10ab2982cc28",
		analytics.WithInitialUserSourcing(session))

	defer sakari.Close()

	trackPage(sakari)
	trackArbitraryEvent(sakari)
	trackGroupEvent_By_Context(sakari)
	trackIdentify(sakari)
}

func trackPage(sakari *analytics.Client) {
	_ = sakari.Page(&analytics.Page{Name: "Page - X", Category: "x-me"})
}

func trackArbitraryEvent(sakari *analytics.Client) {
	_ = sakari.Track(&analytics.Track{Event: "click",
		Properties: analytics.NewProperties().Set("name", "Paul").Set("duration", 10)})
}

func trackGroupEvent_By_Context(sakari *analytics.Client) {
	_ = sakari.Group(
		&analytics.Group{GroupId: "group-1", Traits: analytics.NewTrait().Set("package", "premium")},
		analytics.WithContext(context.Background()), // optional
	)
}

func trackIdentify(sakari *analytics.Client) {
	_ = sakari.Identify(
		&analytics.Identify{
			UserId:      "user@gmail.com",
			AnonymousId: "user-id",
			Traits:      analytics.NewTrait().Set("package", "premium")},
	)
}
