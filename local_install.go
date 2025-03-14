// Experimental code for install local with dependency refactoring
// Not at feature parity with install.go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jguer/yay/v12/pkg/db"
	"github.com/Jguer/yay/v12/pkg/dep"
	"github.com/Jguer/yay/v12/pkg/multierror"
	"github.com/Jguer/yay/v12/pkg/runtime"
	"github.com/Jguer/yay/v12/pkg/settings"
	"github.com/Jguer/yay/v12/pkg/settings/exe"
	"github.com/Jguer/yay/v12/pkg/settings/parser"
	"github.com/Jguer/yay/v12/pkg/sync"

	gosrc "github.com/Morganamilo/go-srcinfo"
	"github.com/leonelquinteros/gotext"
)

var ErrNoBuildFiles = errors.New(gotext.Get("cannot find PKGBUILD and .SRCINFO in directory"))

func srcinfoExists(ctx context.Context,
	cmdBuilder exe.ICmdBuilder, targetDir string,
) error {
	srcInfoDir := filepath.Join(targetDir, ".SRCINFO")
	pkgbuildDir := filepath.Join(targetDir, "PKGBUILD")
	if _, err := os.Stat(srcInfoDir); err == nil {
		if _, err := os.Stat(pkgbuildDir); err == nil {
			return nil
		}
	}

	if _, err := os.Stat(pkgbuildDir); err == nil {
		// run makepkg to generate .SRCINFO
		srcinfo, stderr, err := cmdBuilder.Capture(cmdBuilder.BuildMakepkgCmd(ctx, targetDir, "--printsrcinfo"))
		if err != nil {
			return fmt.Errorf("unable to generate .SRCINFO: %w - %s", err, stderr)
		}

		if len(srcinfo) == 0 {
			return fmt.Errorf("generated .SRCINFO is empty, check your PKGBUILD for errors")
		}

		if err := os.WriteFile(srcInfoDir, []byte(srcinfo), 0o600); err != nil {
			return fmt.Errorf("unable to write .SRCINFO: %w", err)
		}

		return nil
	}

	return fmt.Errorf("%w: %s", ErrNoBuildFiles, targetDir)
}

func installLocalPKGBUILD(
	ctx context.Context,
	run *runtime.Runtime,
	cmdArgs *parser.Arguments,
	dbExecutor db.Executor,
) error {
	aurCache := run.AURClient
	noCheck := strings.Contains(run.Cfg.MFlags, "--nocheck")

	if len(cmdArgs.Targets) < 1 {
		return errors.New(gotext.Get("no target directories specified"))
	}

	srcInfos := map[string]*gosrc.Srcinfo{}
	for _, targetDir := range cmdArgs.Targets {
		if err := srcinfoExists(ctx, run.CmdBuilder, targetDir); err != nil {
			return err
		}

		pkgbuild, err := gosrc.ParseFile(filepath.Join(targetDir, ".SRCINFO"))
		if err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("failed to parse .SRCINFO"), err)
		}

		srcInfos[targetDir] = pkgbuild
	}

	grapher := dep.NewGrapher(dbExecutor, aurCache, false, settings.NoConfirm,
		cmdArgs.ExistsDouble("d", "nodeps"), noCheck, cmdArgs.ExistsArg("needed"),
		run.Logger.Child("grapher"))
	graph, err := grapher.GraphFromSrcInfos(ctx, nil, srcInfos)
	if err != nil {
		return err
	}

	opService := sync.NewOperationService(ctx, dbExecutor, run)
	multiErr := &multierror.MultiError{}
	targets := graph.TopoSortedLayerMap(func(name string, ii *dep.InstallInfo) error {
		if ii.Source == dep.Missing {
			multiErr.Add(fmt.Errorf("%w: %s %s", ErrPackagesNotFound, name, ii.Version))
		}
		return nil
	})

	if err := multiErr.Return(); err != nil {
		return err
	}
	return opService.Run(ctx, run, cmdArgs, targets, []string{})
}
