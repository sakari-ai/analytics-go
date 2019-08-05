package main

import "github.com/sakari-ai/analytics-go"

// User mock up sourcing
// Ideally, this UserSession service must return user ID via Context
type UserSession struct {
}

func (u *UserSession) GetUserId() string {
	// return your current user id by session base
	return "user-id"
}
func main() {
	session := new(UserSession)
	sakari := analytics.New("uyKDw5O0p0t6IwuWAMtiMixT2vXpoLUH",
		"156c5bcd-2b8b-437c-b58c-10ab2982cc28",
		analytics.WithInitialUserSourcing(session.GetUserId))

	defer sakari.Close()

	trackPage(sakari)
	trackArbitraryEvent(sakari)
}

func trackPage(sakari *analytics.Client) {
	_ = sakari.Page(&analytics.Page{Name: "Page - X", Category: "x-me"})
}

func trackArbitraryEvent(sakari *analytics.Client) {
	_ = sakari.Track(&analytics.Track{Event: "click",
		Properties: analytics.NewProperties().Set("name", "Paul").Set("duration", 10)})
}

func trackGroupEvent(sakari *analytics.Client) {
	_ = sakari.Group(&analytics.Group{GroupId: "group-1", Traits: analytics.NewTrait().Set("package", "premium")})
}
