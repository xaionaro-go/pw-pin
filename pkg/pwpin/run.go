package pwpin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/facebookincubator/go-belt/tool/logger"
)

func (p *SimplePlumber) createLink(ctx context.Context, linkKey LinkKey) (_ret bool, _err error) {
	logger.Debugf(ctx, "createLink(%s)", jsoninze(linkKey))
	defer func() { logger.Tracef(ctx, "/createLink(%s): %v %v", jsoninze(linkKey), _ret, _err) }()

	err := p.runCmd(
		ctx,
		"pw-link",
		fmt.Sprintf("%d", linkKey.From.PortID),
		fmt.Sprintf("%d", linkKey.To.PortID),
	)
	if err != nil {
		runErr := err.(ErrRun)
		stdErrMsg := strings.Trim(runErr.Stderr, " \r\n\t")
		if stdErrMsg == "failed to link ports: File exists" {
			logger.Debugf(ctx, "the link already exists, ignoring the error")
			return false, nil
		}
		return false, fmt.Errorf("failed to create the link %s: %w", jsoninze(linkKey), err)
	}
	return true, nil
}

func (p *SimplePlumber) destroyLink(ctx context.Context, linkKey LinkKey) (_ret bool, _err error) {
	logger.Debugf(ctx, "destroyLink(%s)", jsoninze(linkKey))
	defer func() { logger.Tracef(ctx, "/destroyLink(%s): %v", jsoninze(linkKey), _ret, _err) }()

	err := p.runCmd(
		ctx,
		"pw-link", "-d",
		fmt.Sprintf("%d", linkKey.From.PortID),
		fmt.Sprintf("%d", linkKey.To.PortID),
	)
	if err != nil {
		runErr := err.(ErrRun)
		stdErrMsg := strings.Trim(runErr.Stderr, " \r\n\t")
		if stdErrMsg == "failed to unlink ports: No such file or directory" {
			logger.Debugf(ctx, "the link already does not exist, ignoring the error")
			return false, nil
		}
		return true, fmt.Errorf("failed to destroy the link %s: %w", jsoninze(linkKey), err)
	}
	return true, nil
}

type ErrRun struct {
	Err     error
	Command string
	Args    []string
	Stdout  string
	Stderr  string
}

func (e ErrRun) Error() string {
	return fmt.Sprintf(
		"failed to run command '%s %s': %v\nstdout: %s\nstderr: %s",
		e.Command, strings.Join(e.Args, " "), e.Err, e.Stdout, e.Stderr,
	)
}

func (p *SimplePlumber) runCmd(ctx context.Context, arg0 string, arg1toN ...string) (_err error) {
	logger.Debugf(ctx, "runCmd(%s, %v)", arg0, arg1toN)
	defer func() { logger.Tracef(ctx, "/runCmd(%s, %v), %v", arg0, arg1toN, _err) }()

	if callback := CtxOnRun(ctx); callback != nil {
		callback(ctx, arg0, arg1toN...)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(arg0, arg1toN...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return ErrRun{
			Err:     err,
			Command: arg0,
			Args:    arg1toN,
			Stdout:  stdout.String(),
			Stderr:  stderr.String(),
		}
	}
	return nil
}

type ctxKeyOnRunKey struct{}

type OnRunFunc func(ctx context.Context, arg0 string, arg1toN ...string)

func CtxWithOnRun(ctx context.Context, callback OnRunFunc) context.Context {
	return context.WithValue(ctx, ctxKeyOnRunKey{}, callback)
}

func CtxOnRun(ctx context.Context) OnRunFunc {
	callback, ok := ctx.Value(ctxKeyOnRunKey{}).(OnRunFunc)
	if !ok {
		return func(ctx context.Context, arg0 string, arg1toN ...string) {}
	}
	return callback
}
