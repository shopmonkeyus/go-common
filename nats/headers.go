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
	ContentEncodingHdr = "content-encoding"
)

// getters
func GetRegionFromHeader(m *nats.Msg) string {
	return getHeader(m, RegionHdr)
}

func GetRequestIdFromHeader(m *nats.Msg) string {
	return getHeader(m, RequestIdHdr)
}

func GetCompanyIdFromHeader(m *nats.Msg) string {
	return getHeader(m, CompanyIdHdr)
}

func GetLocationIdFromHeader(m *nats.Msg) string {
	return getHeader(m, LocationIdHdr)
}

func GetUserIdFromHeader(m *nats.Msg) string {
	return getHeader(m, UserIdHdr)
}

func GetSessionIdFromHeader(m *nats.Msg) string {
	return getHeader(m, SessionIdHdr)
}

func GetMsgIdFromHeader(m *nats.Msg) string {
	return getHeader(m, nats.MsgIdHdr)
}

func GetContentEncodingFromHeader(m *nats.Msg) string {
	return getHeader(m, ContentEncodingHdr)
}

func GetCustomHeaderValue(m *nats.Msg, header string) string {
	return getHeader(m, header)
}

func getHeader(m *nats.Msg, header string) string {
	return m.Header.Get(header)
}

// setters
func SetRegionHeader(m *nats.Msg, value string) {
	setHeader(m, RegionHdr, value)
}

func SetRequestIdHeader(m *nats.Msg, value string) {
	setHeader(m, RequestIdHdr, value)
}

func SetCompanyIdHeader(m *nats.Msg, value string) {
	setHeader(m, CompanyIdHdr, value)
}

func SetLocationIdHeader(m *nats.Msg, value string) {
	setHeader(m, LocationIdHdr, value)
}

func SetUserIdHeader(m *nats.Msg, value string) {
	setHeader(m, UserIdHdr, value)
}

func SetSessionIdHeader(m *nats.Msg, value string) {
	setHeader(m, SessionIdHdr, value)
}

func SetMsgIdHeader(m *nats.Msg, value string) {
	setHeader(m, nats.MsgIdHdr, value)
}

func SetContentEncodingHeader(m *nats.Msg, value string) {
	setHeader(m, ContentEncodingHdr, value)
}

func SetCustomHeader(m *nats.Msg, header string, value string) {
	setHeader(m, header, value)
}

func setHeader(m *nats.Msg, header string, value string) {
	m.Header.Set(header, value)
}
