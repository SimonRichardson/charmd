package daemon

// API lists the daemon endpoints included in this minimal implementation.
var API = []*Command{
	{Path: "/v1/changes", GET: v1GetChanges},
	{Path: "/v1/changes/{id}", GET: v1GetChange, POST: v1PostChange},
	{Path: "/v1/changes/{id}/wait", GET: v1GetChangeWait},
	{Path: "/v1/notices", GET: v1GetNotices, POST: v1PostNotices},
	{Path: "/v1/notices/{id}", GET: v1GetNotice},
}
