package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/ingresslabs/torque/internal/helpui"
	"github.com/ingresslabs/torque/internal/logging"
	"github.com/spf13/cobra"
)

type helpUIRunner func(context.Context, *cobra.Command, io.Writer, string, bool) error

var runHelpUI helpUIRunner = runHelpUIServer

func newHelpCommand(root *cobra.Command) *cobra.Command {
	var uiAddr string
	var showAll bool
	cmd := &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(uiAddr) != "" {
				return runHelpUI(cmd.Context(), root, cmd.ErrOrStderr(), uiAddr, showAll)
			}
			target, _, err := cmd.Root().Find(args)
			if err != nil || target == nil {
				return cmd.Root().Help()
			}
			target.SetContext(cmd.Context())
			return target.Help()
		},
	}
	cmd.Flags().StringVar(&uiAddr, "ui", "", "Serve the interactive help UI at this address (e.g. :8080)")
	if flag := cmd.Flags().Lookup("ui"); flag != nil {
		flag.NoOptDefVal = ":8080"
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "Include hidden/internal flags and env vars")
	decorateCommandHelp(cmd, "Help Flags")
	return cmd
}

func installRootHelpUI(root *cobra.Command, uiAddr *string) {
	if root == nil || uiAddr == nil {
		return
	}
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if strings.TrimSpace(*uiAddr) == "" {
			defaultHelp(cmd, args)
			return
		}
		if err := runHelpUI(cmd.Context(), root, cmd.ErrOrStderr(), *uiAddr, false); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
		}
	})
}

func runHelpUIServer(ctx context.Context, root *cobra.Command, errOut io.Writer, uiAddr string, showAll bool) error {
	if root == nil {
		return fmt.Errorf("missing root command")
	}
	if errOut == nil {
		errOut = io.Discard
	}
	logLevel, _ := root.PersistentFlags().GetString("log-level")
	if strings.TrimSpace(logLevel) == "" {
		logLevel = "info"
	}
	logger, err := logging.New(logLevel)
	if err != nil {
		return err
	}
	fmt.Fprintf(errOut, "Serving help UI at %s\n", formatHelpURL(uiAddr))
	var opts []helpui.Option
	if showAll {
		opts = append(opts, helpui.WithAll())
	}
	return helpui.New(uiAddr, root, logger.WithName("help-ui"), opts...).Run(ctx)
}

func formatHelpURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		if strings.TrimSpace(h) == "" || h == "0.0.0.0" || h == "::" {
			host = "127.0.0.1:" + p
		}
	}
	u := url.URL{Scheme: "http", Host: host, Path: "/"}
	return u.String()
}
