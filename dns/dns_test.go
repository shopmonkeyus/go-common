package dns

import (
	"context"
	"testing"
	"time"

	"github.com/shopmonkeyus/go-common/cache"
	"github.com/stretchr/testify/assert"
)

func TestDNSIsValidAndCached(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "app.shopmonkey.cloud")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count := c.Hits("dns:app.shopmonkey.cloud")
	assert.True(t, ok)
	assert.Equal(t, 0, count)

	ok, ip, err = d.Lookup(context.Background(), "app.shopmonkey.cloud")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count = c.Hits("dns:app.shopmonkey.cloud")
	assert.True(t, ok)
	assert.Equal(t, 1, count)
}

func TestDNSDomainIsInvalid(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "adasf123sdasdxc.dsadasdshopmonkey.cloud")
	assert.Error(t, err, "dns lookup failed: Non-Existent Domain")
	assert.False(t, ok)
	assert.Nil(t, ip)
	ok, count := c.Hits("dns:adasf123sdasdxc.dsadasdshopmonkey.cloud")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestDNSLocalHost(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "localhost")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count := c.Hits("dns:localhost")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	ok, ip, err = d.Lookup(context.Background(), "localhost")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count = c.Hits("dns:localhost")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestDNS127(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "127.0.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count := c.Hits("dns:127.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	ok, ip, err = d.Lookup(context.Background(), "127.0.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count = c.Hits("dns:127.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestDNSIPAddressSkipped(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "81.0.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count := c.Hits("dns:81.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	ok, ip, err = d.Lookup(context.Background(), "81.0.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count = c.Hits("dns:81.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestDNSPrivateIPSkipped(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	ok, ip, err := d.Lookup(context.Background(), "10.8.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count := c.Hits("dns:81.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)

	ok, ip, err = d.Lookup(context.Background(), "81.0.0.1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.NotNil(t, ip)
	ok, count = c.Hits("dns:81.0.0.1")
	assert.False(t, ok)
	assert.Equal(t, 0, count)
}

func TestDNSTest(t *testing.T) {
	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c, WithFailIfLocal())
	ok, ip, err := d.Lookup(context.Background(), "customer1.app.localhost.my.company.127.0.0.1.nip.io")
	assert.Error(t, err, ErrInvalidIP)
	assert.False(t, ok)
	assert.Nil(t, ip)
}

func TestInvalidDNSEntries(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
	}{
		{"EmptyHostname", ""},
		{"InvalidCharacters", "invalid!hostname"},
		{"TooLongHostname", "this.is.a.very.long.hostname.that.exceeds.the.maximum.length.allowed.by.the.dns.specification.and.should.therefore.fail.validation"},
		{"HostnameWithSpaces", "hostname with spaces"},
		{"HostnameWithUnderscore", "hostname_with_underscore"},
		{"Unresolved DNS", "bugbounty.dod.network"},
		{"Invalid Hostname", "0xA9.0xFE.0xA9.0xFE"},
		{"Invalid IP Address", "169.254.169.254"},
		{"Invalid IP Address From DNS", "169.254.169.254.nip.io"},
		{"Local IP v6", "[::1]"},
		{"Invalid IP v6", "[::ffff:7f00:1]"},
		{"Invalid Virtual DNS", "localtest.me"},
		{"Invalid Virtual DNS to Private", "spoofed.burpcollaborator.net"},
		{"Docker Host Internal", "host.docker.internal"},
	}

	c := cache.NewInMemory(context.Background(), time.Second)
	defer c.Close()
	d := New(c)
	d.isLocal = false

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, ip, err := d.Lookup(context.Background(), tt.hostname)
			assert.Error(t, err, ErrInvalidIP)
			assert.False(t, ok)
			assert.Nil(t, ip)
		})
	}
}
