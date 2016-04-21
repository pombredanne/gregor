package storage

import "net/url"

// ForceParseTime sets parseTime=true on a DSN URL.
func ForceParseTime(dsn *url.URL) *url.URL {
	query := dsn.Query()
	query.Set("parseTime", "true")
	dsn.RawQuery = query.Encode()
	return dsn
}
