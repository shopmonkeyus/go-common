package nats

import "github.com/nats-io/nats.go"

// Default headers which published in messages from API backend
const (
	RegionHdr     = "region"
	RequestIdHdr  = "x-request-id"
	CompanyIdHdr  = "x-company-id"
	UserIdHdr     = "x-user-id"
	LocationIdHdr = "x-location-id"
	SessionIdHdr  = "x-session-id"
)

// Default nats headers
const (
	ContentEncodingHdr  = "content-encoding"
)

func getRegionFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, RegionHdr)
}

func getRequestIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, RequestIdHdr)
}

func getCompanyIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, CompanyIdHdr)
}

func getUserIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, UserIdHdr)
}

func getLocationIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, LocationIdHdr)
}

func getSessionIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, SessionIdHdr)
}

func getMsgIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, nats.MsgIdHdr)
}

func getContentEncodingFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, ContentEncodingHdr)
}

func getHeaderValue(m *nats.Msg, header string) string {
	return m.Header.Get(header)
}
