package gandi

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stangah/lego/acme"
)

// stagingServer is the Let's Encrypt staging server used by the live test
const stagingServer = "https://acme-staging.api.letsencrypt.org/directory"

// user implements acme.User and is used by the live test
type user struct {
	Email        string
	Registration *acme.RegistrationResource
	key          crypto.PrivateKey
}

func (u *user) GetEmail() string {
	return u.Email
}
func (u *user) GetRegistration() *acme.RegistrationResource {
	return u.Registration
}
func (u *user) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// TestDNSProvider runs Present and CleanUp against a fake Gandi RPC
// Server, whose responses are predetermined for particular requests.
func TestDNSProvider(t *testing.T) {
	fakeAPIKey := "123412341234123412341234"
	fakeKeyAuth := "XXXX"
	provider, err := NewDNSProviderCredentials(fakeAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	regexpDate, err := regexp.Compile(`\[ACME Challenge [^\]:]*:[^\]]*\]`)
	if err != nil {
		t.Fatal(err)
	}
	// start fake RPC server
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "text/xml" {
			t.Fatalf("Content-Type: text/xml header not found")
		}
		req, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		req = regexpDate.ReplaceAllLiteral(
			req, []byte(`[ACME Challenge 01 Jan 16 00:00 +0000]`))
		resp, ok := serverResponses[string(req)]
		if !ok {
			t.Fatalf("Server response for request not found")
		}
		_, err = io.Copy(w, strings.NewReader(resp))
		if err != nil {
			t.Fatal(err)
		}
	}))
	defer fakeServer.Close()
	// define function to override findZoneByFqdn with
	fakeFindZoneByFqdn := func(fqdn string, nameserver []string) (string, error) {
		return "example.com.", nil
	}
	// override gandi endpoint and findZoneByFqdn function
	savedEndpoint, savedFindZoneByFqdn := endpoint, findZoneByFqdn
	defer func() {
		endpoint, findZoneByFqdn = savedEndpoint, savedFindZoneByFqdn
	}()
	endpoint, findZoneByFqdn = fakeServer.URL+"/", fakeFindZoneByFqdn
	// run Present
	err = provider.Present("abc.def.example.com", "", fakeKeyAuth)
	if err != nil {
		t.Fatal(err)
	}
	// run CleanUp
	err = provider.CleanUp("abc.def.example.com", "", fakeKeyAuth)
	if err != nil {
		t.Fatal(err)
	}
}

// TestDNSProviderLive performs a live test to obtain a certificate
// using the Let's Encrypt staging server. It runs provided that both
// the environment variables GANDI_API_KEY and GANDI_TEST_DOMAIN are
// set. Otherwise the test is skipped.
//
// To complete this test, go test must be run with the -timeout=40m
// flag, since the default timeout of 10m is insufficient.
func TestDNSProviderLive(t *testing.T) {
	apiKey := os.Getenv("GANDI_API_KEY")
	domain := os.Getenv("GANDI_TEST_DOMAIN")
	if apiKey == "" || domain == "" {
		t.Skip("skipping live test")
	}
	// create a user.
	const rsaKeySize = 2048
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		t.Fatal(err)
	}
	myUser := user{
		Email: "test@example.com",
		key:   privateKey,
	}
	// create a client using staging server
	client, err := acme.NewClient(stagingServer, &myUser, acme.RSA2048)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewDNSProviderCredentials(apiKey)
	if err != nil {
		t.Fatal(err)
	}
	err = client.SetChallengeProvider(acme.DNS01, provider)
	if err != nil {
		t.Fatal(err)
	}
	client.ExcludeChallenges([]acme.Challenge{acme.HTTP01, acme.TLSSNI01})
	// register and agree tos
	reg, err := client.Register()
	if err != nil {
		t.Fatal(err)
	}
	myUser.Registration = reg
	err = client.AgreeToTOS()
	if err != nil {
		t.Fatal(err)
	}
	// complete the challenge
	bundle := false
	_, failures := client.ObtainCertificate([]string{domain}, bundle, nil, false)
	if len(failures) > 0 {
		t.Fatal(failures)
	}
}

// serverResponses is the XML-RPC Request->Response map used by the
// fake RPC server. It was generated by recording a real RPC session
// which resulted in the successful issue of a cert, and then
// anonymizing the RPC data.
var serverResponses = map[string]string{
	// Present Request->Response 1 (getZoneID)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.info</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <string>example.com.</string>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><struct>
<member>
<name>date_updated</name>
<value><dateTime.iso8601>20160216T16:14:23</dateTime.iso8601></value>
</member>
<member>
<name>date_delete</name>
<value><dateTime.iso8601>20170331T16:04:06</dateTime.iso8601></value>
</member>
<member>
<name>is_premium</name>
<value><boolean>0</boolean></value>
</member>
<member>
<name>date_hold_begin</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>date_registry_end</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>authinfo_expiration_date</name>
<value><dateTime.iso8601>20161211T21:31:20</dateTime.iso8601></value>
</member>
<member>
<name>contacts</name>
<value><struct>
<member>
<name>owner</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>admin</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>bill</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>tech</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>reseller</name>
<value><nil/></value></member>
</struct></value>
</member>
<member>
<name>nameservers</name>
<value><array><data>
<value><string>a.dns.gandi.net</string></value>
<value><string>b.dns.gandi.net</string></value>
<value><string>c.dns.gandi.net</string></value>
</data></array></value>
</member>
<member>
<name>date_restore_end</name>
<value><dateTime.iso8601>20170501T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>id</name>
<value><int>2222222</int></value>
</member>
<member>
<name>authinfo</name>
<value><string>ABCDABCDAB</string></value>
</member>
<member>
<name>status</name>
<value><array><data>
<value><string>clientTransferProhibited</string></value>
<value><string>serverTransferProhibited</string></value>
</data></array></value>
</member>
<member>
<name>tags</name>
<value><array><data>
</data></array></value>
</member>
<member>
<name>date_hold_end</name>
<value><dateTime.iso8601>20170401T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>services</name>
<value><array><data>
<value><string>gandidns</string></value>
<value><string>gandimail</string></value>
</data></array></value>
</member>
<member>
<name>date_pending_delete_end</name>
<value><dateTime.iso8601>20170506T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>zone_id</name>
<value><int>1234567</int></value>
</member>
<member>
<name>date_renew_begin</name>
<value><dateTime.iso8601>20120101T00:00:00</dateTime.iso8601></value>
</member>
<member>
<name>fqdn</name>
<value><string>example.com</string></value>
</member>
<member>
<name>autorenew</name>
<value><nil/></value></member>
<member>
<name>date_registry_creation</name>
<value><dateTime.iso8601>20150215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>tld</name>
<value><string>org</string></value>
</member>
<member>
<name>date_created</name>
<value><dateTime.iso8601>20150215T03:04:06</dateTime.iso8601></value>
</member>
</struct></value>
</param>
</params>
</methodResponse>
`,
	// Present Request->Response 2 (cloneZone)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.clone</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <int>1234567</int>
    </value>
  </param>
  <param>
    <value>
      <int>0</int>
    </value>
  </param>
  <param>
    <value>
      <struct>
        <member>
          <name>name</name>
          <value>
            <string>example.com [ACME Challenge 01 Jan 16 00:00 +0000]</string>
          </value>
        </member>
      </struct>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><struct>
<member>
<name>name</name>
<value><string>example.com [ACME Challenge 01 Jan 16 00:00 +0000]</string></value>
</member>
<member>
<name>versions</name>
<value><array><data>
<value><int>1</int></value>
</data></array></value>
</member>
<member>
<name>date_updated</name>
<value><dateTime.iso8601>20160216T16:24:29</dateTime.iso8601></value>
</member>
<member>
<name>id</name>
<value><int>7654321</int></value>
</member>
<member>
<name>owner</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>version</name>
<value><int>1</int></value>
</member>
<member>
<name>domains</name>
<value><int>0</int></value>
</member>
<member>
<name>public</name>
<value><boolean>0</boolean></value>
</member>
</struct></value>
</param>
</params>
</methodResponse>
`,
	// Present Request->Response 3 (newZoneVersion)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.version.new</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <int>7654321</int>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><int>2</int></value>
</param>
</params>
</methodResponse>
`,
	// Present Request->Response 4 (addTXTRecord)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.record.add</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <int>7654321</int>
    </value>
  </param>
  <param>
    <value>
      <int>2</int>
    </value>
  </param>
  <param>
    <value>
      <struct>
        <member>
          <name>type</name>
          <value>
            <string>TXT</string>
          </value>
        </member>
        <member>
          <name>name</name>
          <value>
            <string>_acme-challenge.abc.def</string>
          </value>
        </member>
        <member>
          <name>value</name>
          <value>
            <string>ezRpBPY8wH8djMLYjX2uCKPwiKDkFZ1SFMJ6ZXGlHrQ</string>
          </value>
        </member>
        <member>
          <name>ttl</name>
          <value>
            <int>300</int>
          </value>
        </member>
      </struct>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><struct>
<member>
<name>name</name>
<value><string>_acme-challenge.abc.def</string></value>
</member>
<member>
<name>type</name>
<value><string>TXT</string></value>
</member>
<member>
<name>id</name>
<value><int>333333333</int></value>
</member>
<member>
<name>value</name>
<value><string>"ezRpBPY8wH8djMLYjX2uCKPwiKDkFZ1SFMJ6ZXGlHrQ"</string></value>
</member>
<member>
<name>ttl</name>
<value><int>300</int></value>
</member>
</struct></value>
</param>
</params>
</methodResponse>
`,
	// Present Request->Response 5 (setZoneVersion)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.version.set</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <int>7654321</int>
    </value>
  </param>
  <param>
    <value>
      <int>2</int>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><boolean>1</boolean></value>
</param>
</params>
</methodResponse>
`,
	// Present Request->Response 6 (setZone)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.set</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <string>example.com.</string>
    </value>
  </param>
  <param>
    <value>
      <int>7654321</int>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><struct>
<member>
<name>date_updated</name>
<value><dateTime.iso8601>20160216T16:14:23</dateTime.iso8601></value>
</member>
<member>
<name>date_delete</name>
<value><dateTime.iso8601>20170331T16:04:06</dateTime.iso8601></value>
</member>
<member>
<name>is_premium</name>
<value><boolean>0</boolean></value>
</member>
<member>
<name>date_hold_begin</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>date_registry_end</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>authinfo_expiration_date</name>
<value><dateTime.iso8601>20161211T21:31:20</dateTime.iso8601></value>
</member>
<member>
<name>contacts</name>
<value><struct>
<member>
<name>owner</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>admin</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>bill</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>tech</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>reseller</name>
<value><nil/></value></member>
</struct></value>
</member>
<member>
<name>nameservers</name>
<value><array><data>
<value><string>a.dns.gandi.net</string></value>
<value><string>b.dns.gandi.net</string></value>
<value><string>c.dns.gandi.net</string></value>
</data></array></value>
</member>
<member>
<name>date_restore_end</name>
<value><dateTime.iso8601>20170501T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>id</name>
<value><int>2222222</int></value>
</member>
<member>
<name>authinfo</name>
<value><string>ABCDABCDAB</string></value>
</member>
<member>
<name>status</name>
<value><array><data>
<value><string>clientTransferProhibited</string></value>
<value><string>serverTransferProhibited</string></value>
</data></array></value>
</member>
<member>
<name>tags</name>
<value><array><data>
</data></array></value>
</member>
<member>
<name>date_hold_end</name>
<value><dateTime.iso8601>20170401T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>services</name>
<value><array><data>
<value><string>gandidns</string></value>
<value><string>gandimail</string></value>
</data></array></value>
</member>
<member>
<name>date_pending_delete_end</name>
<value><dateTime.iso8601>20170506T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>zone_id</name>
<value><int>7654321</int></value>
</member>
<member>
<name>date_renew_begin</name>
<value><dateTime.iso8601>20120101T00:00:00</dateTime.iso8601></value>
</member>
<member>
<name>fqdn</name>
<value><string>example.com</string></value>
</member>
<member>
<name>autorenew</name>
<value><nil/></value></member>
<member>
<name>date_registry_creation</name>
<value><dateTime.iso8601>20150215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>tld</name>
<value><string>org</string></value>
</member>
<member>
<name>date_created</name>
<value><dateTime.iso8601>20150215T03:04:06</dateTime.iso8601></value>
</member>
</struct></value>
</param>
</params>
</methodResponse>
`,
	// CleanUp Request->Response 1 (setZone)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.set</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <string>example.com.</string>
    </value>
  </param>
  <param>
    <value>
      <int>1234567</int>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><struct>
<member>
<name>date_updated</name>
<value><dateTime.iso8601>20160216T16:24:38</dateTime.iso8601></value>
</member>
<member>
<name>date_delete</name>
<value><dateTime.iso8601>20170331T16:04:06</dateTime.iso8601></value>
</member>
<member>
<name>is_premium</name>
<value><boolean>0</boolean></value>
</member>
<member>
<name>date_hold_begin</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>date_registry_end</name>
<value><dateTime.iso8601>20170215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>authinfo_expiration_date</name>
<value><dateTime.iso8601>20161211T21:31:20</dateTime.iso8601></value>
</member>
<member>
<name>contacts</name>
<value><struct>
<member>
<name>owner</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>admin</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>bill</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>tech</name>
<value><struct>
<member>
<name>handle</name>
<value><string>LEGO-GANDI</string></value>
</member>
<member>
<name>id</name>
<value><int>111111</int></value>
</member>
</struct></value>
</member>
<member>
<name>reseller</name>
<value><nil/></value></member>
</struct></value>
</member>
<member>
<name>nameservers</name>
<value><array><data>
<value><string>a.dns.gandi.net</string></value>
<value><string>b.dns.gandi.net</string></value>
<value><string>c.dns.gandi.net</string></value>
</data></array></value>
</member>
<member>
<name>date_restore_end</name>
<value><dateTime.iso8601>20170501T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>id</name>
<value><int>2222222</int></value>
</member>
<member>
<name>authinfo</name>
<value><string>ABCDABCDAB</string></value>
</member>
<member>
<name>status</name>
<value><array><data>
<value><string>clientTransferProhibited</string></value>
<value><string>serverTransferProhibited</string></value>
</data></array></value>
</member>
<member>
<name>tags</name>
<value><array><data>
</data></array></value>
</member>
<member>
<name>date_hold_end</name>
<value><dateTime.iso8601>20170401T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>services</name>
<value><array><data>
<value><string>gandidns</string></value>
<value><string>gandimail</string></value>
</data></array></value>
</member>
<member>
<name>date_pending_delete_end</name>
<value><dateTime.iso8601>20170506T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>zone_id</name>
<value><int>1234567</int></value>
</member>
<member>
<name>date_renew_begin</name>
<value><dateTime.iso8601>20120101T00:00:00</dateTime.iso8601></value>
</member>
<member>
<name>fqdn</name>
<value><string>example.com</string></value>
</member>
<member>
<name>autorenew</name>
<value><nil/></value></member>
<member>
<name>date_registry_creation</name>
<value><dateTime.iso8601>20150215T02:04:06</dateTime.iso8601></value>
</member>
<member>
<name>tld</name>
<value><string>org</string></value>
</member>
<member>
<name>date_created</name>
<value><dateTime.iso8601>20150215T03:04:06</dateTime.iso8601></value>
</member>
</struct></value>
</param>
</params>
</methodResponse>
`,
	// CleanUp Request->Response 2 (deleteZone)
	`<?xml version="1.0"?>
<methodCall>
  <methodName>domain.zone.delete</methodName>
  <param>
    <value>
      <string>123412341234123412341234</string>
    </value>
  </param>
  <param>
    <value>
      <int>7654321</int>
    </value>
  </param>
</methodCall>`: `<?xml version='1.0'?>
<methodResponse>
<params>
<param>
<value><boolean>1</boolean></value>
</param>
</params>
</methodResponse>
`,
}
