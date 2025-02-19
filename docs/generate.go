package main

import (
	"log"
	"os"

	"github.com/docker/buildx/commands"
	clidocstool "github.com/docker/cli-docs-tool"
	"github.com/docker/cli/cli/command"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	// import drivers otherwise factories are empty
	// for --driver output flag usage
	_ "github.com/docker/buildx/driver/docker"
	_ "github.com/docker/buildx/driver/docker-container"
	_ "github.com/docker/buildx/driver/kubernetes"
)

const defaultSourcePath = "docs/reference/"

type options struct {
	source  string
	formats []string
}

func gen(opts *options) error {
	log.SetFlags(0)

	dockerCLI, err := command.NewDockerCli()
	if err != nil {
		return err
	}
	cmd := &cobra.Command{
		Use:               "docker [OPTIONS] COMMAND [ARG...]",
		Short:             "The base command for the Docker CLI.",
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(commands.NewRootCmd("buildx", true, dockerCLI))

	c, err := clidocstool.New(clidocstool.Options{
		Root:      cmd,
		SourceDir: opts.source,
		Plugin:    true,
	})
	if err != nil {
		return err
	}

	for _, format := range opts.formats {
		switch format {
		case "md":
			if err = c.GenMarkdownTree(cmd); err != nil {
				return err
			}
		case "yaml":
			if err = c.GenYamlTree(cmd); err != nil {
				return err
			}
		default:
			return errors.Errorf("unknown format %q", format)
		}
	}

	return nil
}

func run() error {
	opts := &options{}
	flags := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	flags.StringVar(&opts.source, "source", defaultSourcePath, "Docs source folder")
	flags.StringSliceVar(&opts.formats, "formats", []string{}, "Format (md, yaml)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if len(opts.formats) == 0 {
		return errors.New("Docs format required")
	}
	return gen(opts)
}

func main() {
	if err := run(); err != nil {
		log.Printf("ERROR: %+v", err)
		os.Exit(1)
	}
}
