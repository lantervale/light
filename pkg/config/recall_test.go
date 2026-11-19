package config_test

import (
	"testing"

	"github.com/lantervale/light/pkg/config"
)

func TestTheSitesNeverRememberedAreKeptAsHosts(t *testing.T) {
	if config.Default().RecallExcluded != "" {
		t.Fatal("a site is excluded by default")
	}
	c := config.Default()
	c.RecallExcluded = " https://www.Bank.example/accounts,mail.example:8443  bank.example, no_host !, ,127.0.0.1"
	c = c.Valid()
	if c.RecallExcluded != "bank.example, mail.example, 127.0.0.1" {
		t.Fatalf("the sites are %q", c.RecallExcluded)
	}
	for site, want := range map[string]bool{
		"bank.example": true, "online.bank.example": true, "mybank.example": false, "example": false,
		"127.0.0.1": true, "": false,
	} {
		if c.Excludes(site) != want {
			t.Errorf("%q excluded is %v", site, !want)
		}
	}
	if config.Default().Excludes("bank.example") || config.Default().Excludes("") {
		t.Fatal("with no site named one is excluded")
	}
	if c = c.Excluding("News.example"); c.RecallExcluded != "bank.example, mail.example, 127.0.0.1, news.example" {
		t.Fatalf("with one more the sites are %q", c.RecallExcluded)
	}
	if again := c.Excluding("news.example"); again != c {
		t.Fatalf("excluded twice the sites are %q", again.RecallExcluded)
	}
	data, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if read, unread := config.Decode(data); unread != nil || read != c {
		t.Fatalf("read again the settings are %+v, %v", read, unread)
	}
}
