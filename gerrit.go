package gerrittest

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/crewjam/errset"
	"github.com/opalmer/dockertest"
	"golang.org/x/crypto/ssh"
)

// ProjectName is used anywhere we need a default value (temp files, default
// field values, etc.
const ProjectName = "gerrittest"

// Gerrit is the central struct which combines multiple components
// of the gerrittest project. Use New() to construct this struct.
type Gerrit struct {
	log        *log.Entry
	Config     *Config          `json:"config"`
	Container  *Container       `json:"container"`
	HTTP       *HTTPClient      `json:"-"`
	HTTPPort   *dockertest.Port `json:"http"`
	SSH        *SSHClient       `json:"-"`
	SSHPort    *dockertest.Port `json:"ssh"`
	Repo       *Repository      `json:"repo"`
	PrivateKey ssh.Signer       `json:"-"`
	PublicKey  ssh.PublicKey    `json:"-"`
}

func (g *Gerrit) errLog(logger *log.Entry, err error) error {
	logger.WithError(err).Error()
	return err
}

// startContainer starts the docker container containing Gerrit.
func (g *Gerrit) startContainer() error {
	logger := g.log.WithFields(log.Fields{
		"phase": "setup",
		"task":  "start-container",
	})
	logger.Debug()
	container, err := NewContainer(
		g.Config.Context, g.Config.PortHTTP, g.Config.PortSSH, g.Config.Image)
	if err != nil {
		logger.WithError(err).Error()
		return err
	}

	// Cookies are set based on hostname so we need to be
	// consistent and use 'localhost' if we're working with
	// 127.0.0.1.
	if container.HTTP.Address == "127.0.0.1" {
		container.HTTP.Address = "localhost"
	}

	g.Container = container
	g.SSHPort = container.SSH
	g.HTTPPort = container.HTTP

	return nil
}

// setupSSHKey loads or generates an SSH key.
func (g *Gerrit) setupSSHKey() error {
	logger := g.log.WithFields(log.Fields{
		"phase": "setup",
		"task":  "ssh-key",
	})
	logger.Debug()
	g.Config.CleanupPrivateKey = false

	if g.Config.PrivateKeyPath != "" {
		entry := logger.WithFields(log.Fields{
			"action": "read",
			"path":   g.Config.PrivateKeyPath,
		})
		entry.Debug()
		public, private, err := ReadSSHKeys(g.Config.PrivateKeyPath)
		if err != nil {
			entry.WithError(err).Error()
			return err
		}
		g.Config.GitConfig["core.sshCommand"] = fmt.Sprintf(
			"ssh -i %s -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no", g.Config.PrivateKeyPath)
		g.PrivateKey = private
		g.PublicKey = public
		return nil
	}
	entry := logger.WithFields(log.Fields{
		"action": "generate",
	})

	private, err := GenerateRSAKey()
	if err != nil {
		return g.errLog(entry, err)
	}

	file, err := ioutil.TempFile("", fmt.Sprintf("%s-id_rsa-", ProjectName))
	entry = entry.WithField("path", file.Name())
	if err != nil {
		return g.errLog(entry, err)
	}

	defer file.Close() // nolint: errcheck
	if err := WriteRSAKey(private, file); err != nil {
		return g.errLog(entry, err)
	}

	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return g.errLog(entry, err)
	}
	g.PrivateKey = signer
	g.PublicKey = signer.PublicKey()
	g.Config.PrivateKeyPath = file.Name()
	g.Config.CleanupPrivateKey = true
	g.Config.GitConfig["core.sshCommand"] = fmt.Sprintf(
		"ssh -i %s -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no", g.Config.PrivateKeyPath)
	return nil
}

func (g *Gerrit) setupHTTPClient() error {
	logger := g.log.WithFields(log.Fields{
		"phase": "setup",
		"task":  "http-client",
	})

	client, err := NewHTTPClient(g.Config, g.HTTPPort)
	if err != nil {
		return g.errLog(logger, err)
	}

	g.HTTP = client

	logger.WithField("action", "login").Debug()
	if err := g.HTTP.login(); err != nil {
		return g.errLog(logger, err)
	}

	logger.WithField("action", "insert-key").Debug()
	if err := g.HTTP.insertPublicKey(g.PublicKey); err != nil {
		return g.errLog(logger, err)
	}

	// Generate or set the password.
	if g.Config.Password != "" {
		logger = logger.WithField("action", "set-password")
		logger.Debug()

		if _, err := g.HTTP.Gerrit(); err != nil {
			if err := g.HTTP.setPassword(g.Config.Password); err != nil {
				return g.errLog(logger, err)
			}
		}
	} else {
		logger = logger.WithField("action", "generate-password")
		logger.Debug()
		generated, err := g.HTTP.generatePassword()
		if err != nil {
			return g.errLog(logger, err)
		}
		g.Config.Password = generated
	}

	// Create the gerrit project.
	if g.Config.Project != "" {
		logger = logger.WithFields(log.Fields{
			"action":  "create-project",
			"project": g.Config.Project,
		})
		gerrit, err := client.Gerrit()
		if err != nil {
			return g.errLog(logger, err)
		}
		logger.Debug()
		if _, _, err := gerrit.Projects.CreateProject(g.Config.Project, nil); err != nil {
			return g.errLog(logger, err)
		}
	}

	return err
}

func (g *Gerrit) setupSSHClient() error {
	logger := g.log.WithFields(log.Fields{
		"phase": "setup",
		"task":  "ssh-client",
	})
	logger.Debug()

	client, err := NewSSHClient(g.Config, g.SSHPort)
	if err != nil {
		logger.WithError(err).Error()
		return err
	}

	g.SSH = client
	return nil
}

func (g *Gerrit) setupRepo() error {
	logger := g.log.WithFields(log.Fields{
		"phase": "setup",
		"task":  "repo",
	})
	logger.Debug()

	if g.Config.RepoRoot == "" {
		g.Config.CleanupGitRepo = true
		tmppath, err := ioutil.TempDir("", fmt.Sprintf("%s-", ProjectName))
		if err != nil {
			return err
		}
		g.Config.RepoRoot = tmppath
	}

	repo, err := NewRepository(g.Config)
	if err != nil {
		return err
	}
	g.Repo = repo

	if g.Config.Project != "" {
		return g.Repo.AddRemoteFromContainer(
			g.Container, g.Config.OriginName, g.Config.Project)
	}

	return nil
}

// WriteJSONFile takes the current struct and writes the data to disk
// as json.
func (g *Gerrit) WriteJSONFile(path string) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0600)
}

// Destroy will destroy the container and all associated resources. Custom
// private keys or repositories will not be cleaned up.
func (g *Gerrit) Destroy() error {
	g.log.WithField("phase", "destroy").Debug()
	errs := errset.ErrSet{}
	if g.SSH != nil {
		errs = append(errs, g.SSH.Close())
	}
	if g.Config.CleanupContainer && g.Container != nil {
		errs = append(errs, g.Container.Terminate())
	}
	if g.Config.CleanupGitRepo && g.Repo != nil {
		errs = append(errs, g.Repo.Remove())
	}
	if g.Config.CleanupPrivateKey && g.Config.PrivateKeyPath != "" {
		errs = append(errs, os.Remove(g.Config.PrivateKeyPath))
	}
	return errs.ReturnValue()
}

// New constructs and returns a *Gerrit struct after all setup steps have
// been completed. Once this function returns Gerrit will be running in
// a container, an admin user will be created and a git repository will
// be setup pointing at the service in the container.
func New(cfg *Config) (*Gerrit, error) {
	username := cfg.Username
	if username == "" {
		username = "admin"
	}

	if cfg.Context == nil {
		cfg.Context = context.Background()
	}

	gerrit := &Gerrit{
		log:    log.WithField("cmp", "core"),
		Config: cfg,
	}
	if err := gerrit.setupSSHKey(); err != nil {
		return gerrit, err
	}
	if err := gerrit.startContainer(); err != nil {
		return gerrit, err
	}

	if cfg.SkipSetup {
		return gerrit, nil
	}

	if err := gerrit.setupHTTPClient(); err != nil {
		return gerrit, err
	}
	if err := gerrit.setupSSHClient(); err != nil {
		return gerrit, err
	}
	if err := gerrit.setupRepo(); err != nil {
		return gerrit, err
	}

	return gerrit, nil
}

// NewFromJSON reads information from a json file and returns a *Gerrit
// struct.
func NewFromJSON(path string) (*Gerrit, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	gerrit := &Gerrit{
		log: log.WithField("cmp", "core"),
	}
	if err := json.Unmarshal(data, gerrit); err != nil {
		return nil, err
	}
	gerrit.Config.Context = ctx
	gerrit.Container.ctx = ctx

	docker, err := dockertest.NewClient()
	if err != nil {
		return nil, err
	}
	gerrit.Container.Docker = docker

	repo, err := NewRepository(gerrit.Config)
	if err != nil {
		return nil, err
	}
	gerrit.Repo = repo

	sshClient, err := NewSSHClient(gerrit.Config, gerrit.SSHPort)
	if err != nil {
		return nil, err
	}
	gerrit.SSH = sshClient

	httpClient, err := NewHTTPClient(gerrit.Config, gerrit.HTTPPort)
	if err != nil {
		return nil, err
	}
	gerrit.HTTP = httpClient

	return gerrit, nil
}
