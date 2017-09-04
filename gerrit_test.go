package gerrittest

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/opalmer/dockertest"
	"golang.org/x/crypto/ssh"
	. "gopkg.in/check.v1"
)

type GerritTest struct{}

var _ = Suite(&GerritTest{})

func (s *GerritTest) gerrit(c *C) *Gerrit {
	g := &Gerrit{
		Config: NewConfig(),
		log:    log.WithField("cmp", "core"),
	}
	g.Config.Username = "admin"
	g.Username = g.Config.Username
	return g
}

func (s *GerritTest) serverToPort(c *C, server *httptest.Server) *dockertest.Port {
	split := strings.Split(server.Listener.Addr().String(), ":")
	port, err := strconv.ParseUint(split[1], 10, 16)
	c.Assert(err, IsNil)
	return &dockertest.Port{
		Address: split[0],
		Public:  uint16(port),
	}
}

func (s *GerritTest) addSSHKey(c *C, g *Gerrit) string {
	file, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	key, err := GenerateRSAKey()
	c.Assert(err, IsNil)
	c.Assert(WriteRSAKey(key, file), IsNil)
	g.Config.PrivateKey = file.Name()
	signer, err := ssh.NewSignerFromKey(key)
	c.Assert(err, IsNil)
	g.PublicKey = signer.PublicKey()
	return file.Name()
}

func (s *GerritTest) TestNew(c *C) {
	if testing.Short() {
		c.Skip("-short set")
	}
	cfg := NewConfig()
	gerrit, err := New(cfg)
	c.Assert(err, IsNil)
	c.Assert(gerrit.Destroy(), IsNil)
}

func (s *GerritTest) TestGerrit_setupSSHKey_noPrivateKey(c *C) {
	g := s.gerrit(c)
	defer os.Remove(s.addSSHKey(c, g)) // nolint: errcheck
	c.Assert(g.setupSSHKey(), IsNil)
}

func (s *GerritTest) TestGerrit_setupHTTPClient_passwordSet(c *C) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests {
		case 0:
			w.WriteHeader(http.StatusOK)
		case 1:
			w.WriteHeader(http.StatusCreated)
		}
		requests++
	}))

	defer ts.Close()
	g := s.gerrit(c)
	defer os.Remove(s.addSSHKey(c, g)) // nolint: errcheck
	g.HTTPPort = s.serverToPort(c, ts)
	g.Password = "foo"
	c.Assert(g.setupHTTPClient(), IsNil)
}

func (s *GerritTest) TestGerrit_setupHTTPClient_errLogin(c *C) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests {
		case 0:
			w.WriteHeader(http.StatusOK)
		case 1:
			w.WriteHeader(http.StatusBadRequest)
		}
		requests++
	}))

	defer ts.Close()
	g := s.gerrit(c)
	defer os.Remove(s.addSSHKey(c, g)) // nolint: errcheck
	g.HTTPPort = s.serverToPort(c, ts)
	c.Assert(g.setupHTTPClient(), ErrorMatches, "Response code 400 != 201")
}

func (s *GerritTest) TestGerrit_setupHTTPClient_generatePassword(c *C) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests {
		case 0:
			w.WriteHeader(http.StatusOK)
		case 1:
			w.WriteHeader(http.StatusCreated)
		default:
			fmt.Fprint(w, "'Hello'")
		}
		requests++
	}))

	defer ts.Close()
	g := s.gerrit(c)
	defer os.Remove(s.addSSHKey(c, g)) // nolint: errcheck
	g.HTTPPort = s.serverToPort(c, ts)
	c.Assert(g.setupHTTPClient(), IsNil)
}

func (s *GerritTest) TestGerrit_setupHTTPClient_errGeneratePassword(c *C) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests {
		case 0:
			w.WriteHeader(http.StatusOK)
		case 1:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusBadRequest)
			//fmt.Fprint(w, "'Hello'")
		}
		requests++
	}))

	defer ts.Close()
	g := s.gerrit(c)
	defer os.Remove(s.addSSHKey(c, g)) // nolint: errcheck
	g.HTTPPort = s.serverToPort(c, ts)
	c.Assert(g.setupHTTPClient(), ErrorMatches, "Response code 400 != 200")
}

func (s *GerritTest) TestGerrit_setupHTTPClient_errUsernameNotProvided(c *C) {
	g := s.gerrit(c)
	g.Username = ""
	c.Assert(g.setupHTTPClient(), ErrorMatches, "Username not provided")
}
