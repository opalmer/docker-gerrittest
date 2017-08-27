package gerrittest

import (
	"os/exec"

	log "github.com/Sirupsen/logrus"
)

// Diff is a struct which represents a single commit to the
// repository.
type Diff struct {
	// Error should be set whenever
	Error   error
	Content []byte
	Commit  string
}

func (d *Diff) errLog(entry *log.Entry, err error) error {
	entry.WithError(err).Error()
	return err
}

// ApplyToRoot will apply the given diff to the provided repository root.
func (d *Diff) ApplyToRoot(root string) error {
	if d.Error != nil {
		return d.Error
	}
	logger := log.WithFields(log.Fields{
		"phase":  "apply-diff",
		"patch":  string(d.Content),
		"action": "apply-to-root",
	})
	cmd := exec.Command(
		"git", "-C", root, "apply", "-v")
	writer, err := cmd.StdinPipe()
	if err != nil {
		return d.errLog(logger, err)
	}
	if _, err := writer.Write(d.Content); err != nil {
		return d.errLog(logger, err)
	}
	if err := writer.Close(); err != nil {
		return d.errLog(logger, err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		logger = logger.WithField("output", string(out))
		return d.errLog(logger, err)
	}
	return nil
}

// ApplyToRepository takes the diff and applies it to the given
// repository using ApplyToRoot() after which this function will amend
// the current commit. It's up to the caller to push the change.
func (d *Diff) ApplyToRepository(repo *Repository) error {
	return nil
}
