package account4399

type SDKData struct {
	UID      string
	Token    string
	Username string
	Time     string
}

type CookieResult struct {
	Cookie       string
	SDKUID       string
	SessionID    string
	LoginChannel string
}

type SessionCookies struct {
	Uauth string
	Puser string
}
