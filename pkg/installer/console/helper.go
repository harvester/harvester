package console

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/jroimartin/gocui"
	yipSchema "github.com/rancher/yip/pkg/schema"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/util"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

const loginUser = "rancher"

type passwordWrapper struct {
	c                *Console
	passwordV        *widgets.Input
	passwordConfirmV *widgets.Input
}

func (p *passwordWrapper) passwordVConfirmKeyBinding(_ *gocui.Gui, _ *gocui.View) error {
	password1V, err := p.c.GetElement(passwordPanel)
	if err != nil {
		return err
	}
	userInputData.Password, err = password1V.GetData()
	if err != nil {
		return err
	}
	if userInputData.Password == "" {
		return p.c.setContentByName(validatorPanel, "Password is required")
	}
	return showNext(p.c, passwordConfirmPanel)
}

func (p *passwordWrapper) passwordVEscapeKeyBinding(_ *gocui.Gui, _ *gocui.View) error {
	var err error
	if err = p.passwordV.Close(); err != nil {
		return err
	}
	if err = p.passwordConfirmV.Close(); err != nil {
		return err
	}
	if err := p.c.setContentByName(notePanel, ""); err != nil {
		return err
	}
	if p.c.config.Install.Mode == config.ModeJoin {
		return showNext(p.c, askRolePanel)
	}
	return showNext(p.c, askCreatePanel)
}

func (p *passwordWrapper) passwordConfirmVArrowUpKeyBinding(_ *gocui.Gui, _ *gocui.View) error {
	var err error
	userInputData.PasswordConfirm, err = p.passwordConfirmV.GetData()
	if err != nil {
		return err
	}
	return showNext(p.c, passwordPanel)
}

func (p *passwordWrapper) passwordConfirmVKeyEnter(_ *gocui.Gui, _ *gocui.View) error {
	var err error
	userInputData.PasswordConfirm, err = p.passwordConfirmV.GetData()
	if err != nil {
		return err
	}
	if userInputData.Password != userInputData.PasswordConfirm {
		return p.c.setContentByName(validatorPanel, "Password mismatching")
	}
	if err = p.passwordV.Close(); err != nil {
		return err
	}
	if err = p.passwordConfirmV.Close(); err != nil {
		return err
	}
	encrypted, err := util.GetEncryptedPasswd(userInputData.Password)
	if err != nil {
		return err
	}
	p.c.config.Password = encrypted
	out, err := applyPassword(loginUser, encrypted)
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	logrus.Infof("Default user password updated: %s", out)
	return showDiskPage(p.c)
}

func (p *passwordWrapper) passwordConfirmVKeyEscape(_ *gocui.Gui, _ *gocui.View) error {
	var err error
	if err = p.passwordV.Close(); err != nil {
		return err
	}
	if err = p.passwordConfirmV.Close(); err != nil {
		return err
	}
	if err := p.c.setContentByName(notePanel, ""); err != nil {
		return err
	}
	if p.c.config.Install.Mode == config.ModeJoin {
		return showNext(p.c, askRolePanel)
	}
	return showNext(p.c, askCreatePanel)
}

func applyPassword(username, passwordHash string) ([]byte, error) {
	user := yipSchema.User{
		Name:         username,
		PasswordHash: passwordHash,
	}
	if !user.Exists() {
		return []byte(fmt.Sprintf("user %s doesn't exist.. skip password configuration", user.Name)), nil
	}

	conf := &yipSchema.YipConfig{
		Name: "User Password Configuration",
		Stages: map[string][]yipSchema.Stage{
			"live": {
				yipSchema.Stage{
					Users: map[string]yipSchema.User{
						username: user,
					},
				},
			},
		},
	}
	bytes, err := yaml.Marshal(conf)
	if err != nil {
		return nil, err
	}

	liveFile, err := os.CreateTemp("/tmp", "live.XXXXXXXX")
	if err != nil {
		return nil, err
	}
	defer os.Remove(liveFile.Name()) //nolint:errcheck
	if _, err := liveFile.Write(bytes); err != nil {
		return nil, err
	}

	// #nosec G204
	cmd := exec.Command("/usr/bin/yip", "-s", "live", liveFile.Name())
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}
