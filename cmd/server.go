package cmd

import (
	"fmt"
	"os"

	"regexp"
	"strings"

	"github.com/hootsuite/atlantis/server"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// To add a new flag you must:
// 1. Add a const with the flag name (in alphabetic order).
// 2. Add a new field to server.Config and set the mapstructure tag equal to the flag name.
// 3. Add your flag's description etc. to the stringFlags, intFlags, or boolFlags slices.
const (
	AtlantisURLFlag             = "atlantis-url"
	ApprovalURLFlag             = "approval-url"
	ConfigFlag                  = "config"
	DataDirFlag                 = "data-dir"
	GHHostnameFlag              = "gh-hostname"
	GHTokenFlag                 = "gh-token"
	GHUserFlag                  = "gh-user"
	GHWebHookSecret             = "gh-webhook-secret"
	GitlabHostnameFlag          = "gitlab-hostname"
	GitlabTokenFlag             = "gitlab-token"
	GitlabUserFlag              = "gitlab-user"
	GitlabWebHookSecret         = "gitlab-webhook-secret"
	LogLevelFlag                = "log-level"
	PortFlag                    = "port"
	RequireApprovalFlag         = "require-approval"
	RequireExternalApprovalFlag = "require-external-approval"
	EnvDetectionWorkflow        = "environment-detection-workflow"
	GitFlowEnvDir               = "gitflow-environment-dir"
	GitFlowEnvBranchMap         = "gitflow-environment-branch-map"
)

var stringFlags = []stringFlag{
	{
		name:        AtlantisURLFlag,
		description: "URL that Atlantis can be reached at. Defaults to http://$(hostname):$port where $port is from --" + PortFlag + ".",
	},
	{
		name:        ApprovalURLFlag,
		description: "URL for approval endpoint.",
	},
	{
		name:        ConfigFlag,
		description: "Path to config file.",
	},
	{
		name:        DataDirFlag,
		description: "Path to directory to store Atlantis data.",
		value:       "~/.atlantis",
	},
	{
		name:        GHHostnameFlag,
		description: "Hostname of your Github Enterprise installation. If using github.com, no need to set.",
		value:       "github.com",
	},
	{
		name:        GHUserFlag,
		description: "GitHub username of API user.",
	},
	{
		name:        GHTokenFlag,
		description: "GitHub token of API user. Can also be specified via the ATLANTIS_GH_TOKEN environment variable.",
		env:         "ATLANTIS_GH_TOKEN",
	},
	{
		name: GHWebHookSecret,
		description: "Optional secret used to validate GitHub webhooks (see https://developer.github.com/webhooks/securing/)." +
			" If not specified, Atlantis won't be able to validate that the incoming webhook call came from GitHub. " +
			"Can also be specified via the ATLANTIS_GH_WEBHOOK_SECRET environment variable.",
		env: "ATLANTIS_GH_WEBHOOK_SECRET",
	},
	{
		name:        GitlabHostnameFlag,
		description: "Hostname of your GitLab Enterprise installation. If using gitlab.com, no need to set.",
		value:       "gitlab.com",
	},
	{
		name:        GitlabUserFlag,
		description: "GitLab username of API user.",
	},
	{
		name:        GitlabTokenFlag,
		description: "GitLab token of API user. Can also be specified via the ATLANTIS_GITLAB_TOKEN environment variable.",
		env:         "ATLANTIS_GITLAB_TOKEN",
	},
	{
		name: GitlabWebHookSecret,
		description: "Optional secret used to validate GitLab webhooks." +
			" If not specified, Atlantis won't be able to validate that the incoming webhook call came from GitLab. " +
			"Can also be specified via the ATLANTIS_GITLAB_WEBHOOK_SECRET environment variable.",
		env: "ATLANTIS_GITLAB_WEBHOOK_SECRET",
	},
	{
		name: GitFlowEnvDir,
		description: "Directory relative to the repo root which holds the environment configuration. Leave empty to reference the root dir" +
			"Can also be specified via the ATLANTIS_GITFLOW_ENV_DIR environment variable",
		env: "ATLANTIS_GITFLOW_ENV_DIR",
	},
	{
		name: EnvDetectionWorkflow,
		description: "Select how atlantis should determine the environment to execute. Either modifiedfiles or gitflow" +
			"Can also be specified via the ATLANTIS_ENV_DETECTION_WORKFLOW environment variable",
		env:   "ATLANTIS_ENV_DETECTION_WORKFLOW",
		value: "modifiedfiles",
	},
	{
		name:        LogLevelFlag,
		description: "Log level. Either debug, info, warn, or error.",
		value:       "info",
	},
}
var boolFlags = []boolFlag{
	{
		name:        RequireApprovalFlag,
		description: "Require pull requests to be \"Approved\" before allowing the apply command to be run.",
		value:       false,
	},
	{
		name:        RequireExternalApprovalFlag,
		description: "Require external approval for pull requests.",
		value:       false,
	},
}
var intFlags = []intFlag{
	{
		name:        PortFlag,
		description: "Port to bind to.",
		value:       4141,
	},
}

var stringSetFlags = []stringSetFlag{
	stringSetFlag{
		name:        GitFlowEnvBranchMap,
		description: "A list of environment to branch mappings in the form of prod:master",
	},
}

type stringFlag struct {
	name        string
	description string
	value       string
	env         string
}
type intFlag struct {
	name        string
	description string
	value       int
}
type boolFlag struct {
	name        string
	description string
	value       bool
}
type stringSetFlag struct {
	name        string
	description string
	value       []string
	env         string
}

// ServerCmd is an abstraction that helps us test. It allows
// us to mock out starting the actual server.
type ServerCmd struct {
	ServerCreator ServerCreator
	Viper         *viper.Viper
	// SilenceOutput set to true means nothing gets printed.
	// Useful for testing to keep the logs clean.
	SilenceOutput bool
}

// ServerCreator creates servers.
// It's an abstraction to help us test.
type ServerCreator interface {
	NewServer(config server.Config) (ServerStarter, error)
}

// DefaultServerCreator is the concrete implementation of ServerCreator.
type DefaultServerCreator struct{}

// ServerStarter is for starting up a server.
// It's an abstraction to help us test.
type ServerStarter interface {
	Start() error
}

// NewServer returns the real Atlantis server object.
func (d *DefaultServerCreator) NewServer(config server.Config) (ServerStarter, error) {
	return server.NewServer(config)
}

// Init returns the runnable cobra command.
func (s *ServerCmd) Init() *cobra.Command {
	c := &cobra.Command{
		Use:   "server",
		Short: "Start the atlantis server",
		Long: `Start the atlantis server

Flags can also be set in a yaml config file (see --` + ConfigFlag + `).
Config file values are overridden by environment variables which in turn are overridden by flags.`,
		SilenceErrors: true,
		SilenceUsage:  s.SilenceOutput,
		PreRunE: s.withErrPrint(func(cmd *cobra.Command, args []string) error {
			return s.preRun()
		}),
		RunE: s.withErrPrint(func(cmd *cobra.Command, args []string) error {
			return s.run()
		}),
	}

	// If a user passes in an invalid flag, tell them what the flag was.
	c.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		fmt.Fprintf(os.Stderr, "\033[31mError: %s\033[39m\n\n", err.Error())
		return err
	})

	// Set string flags.
	for _, f := range stringFlags {
		c.Flags().String(f.name, f.value, f.description)
		if f.env != "" {
			s.Viper.BindEnv(f.name, f.env) // nolint: errcheck
		}
		s.Viper.BindPFlag(f.name, c.Flags().Lookup(f.name)) // nolint: errcheck
	}

	// Set int flags.
	for _, f := range intFlags {
		c.Flags().Int(f.name, f.value, f.description)
		s.Viper.BindPFlag(f.name, c.Flags().Lookup(f.name)) // nolint: errcheck
	}

	// Set bool flags.
	for _, f := range boolFlags {
		c.Flags().Bool(f.name, f.value, f.description)
		s.Viper.BindPFlag(f.name, c.Flags().Lookup(f.name)) // nolint: errcheck
	}

	// Set stringsetflags flags.
	for _, f := range stringSetFlags {
		c.Flags().StringSlice(f.name, f.value, f.description)
		s.Viper.BindPFlag(f.name, c.Flags().Lookup(f.name))
	}

	return c
}

func (s *ServerCmd) preRun() error {
	// If passed a config file then try and load it.
	configFile := s.Viper.GetString(ConfigFlag)
	if configFile != "" {
		s.Viper.SetConfigFile(configFile)
		if err := s.Viper.ReadInConfig(); err != nil {
			return errors.Wrapf(err, "invalid config: reading %s", configFile)
		}
	}
	return nil
}

func (s *ServerCmd) run() error {
	var config server.Config
	if err := s.Viper.Unmarshal(&config); err != nil {
		return err
	}
	if err := validate(config); err != nil {
		return err
	}
	if err := setAtlantisURL(&config); err != nil {
		return err
	}
	if err := setDataDir(&config); err != nil {
		return err
	}
	trimAtSymbolFromUsers(&config)

	// Config looks good. Start the server.
	server, err := s.ServerCreator.NewServer(config)
	if err != nil {
		return errors.Wrap(err, "initializing server")
	}
	return server.Start()
}

func validate(config server.Config) error {
	logLevel := config.LogLevel
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		return errors.New("invalid log level: not one of debug, info, warn, error")
	}
	vcsErr := fmt.Errorf("--%s/--%s or --%s/--%s must be set", GHUserFlag, GHTokenFlag, GitlabUserFlag, GitlabTokenFlag)

	// The following combinations are valid.
	// 1. github user and token
	// 2. gitlab user and token
	// 3. all 4 set
	// We validate using contradiction (I think).
	if config.GithubUser != "" && config.GithubToken == "" || config.GithubToken != "" && config.GithubUser == "" {
		return vcsErr
	}
	if config.GitlabUser != "" && config.GitlabToken == "" || config.GitlabToken != "" && config.GitlabUser == "" {
		return vcsErr
	}
	// At this point, we know that there can't be a single user/token without
	// its pair, but we haven't checked if any user/token is set at all.
	if config.GithubUser == "" && config.GitlabUser == "" {
		return vcsErr
	}

	// Check if EnvDetectionWorkflow is set correctly
	envDW := config.EnvDetectionWorkflow
	if envDW != "modifiedfiles" && envDW != "gitflow" {
		return errors.New("invalid env detection workflow: not one of modifiedfiles, gitflow")
	}

	// Check if GitFlowEnvDirMapping has the correct syntax
	sep := regexp.MustCompile(":")
	for _, val := range config.GitflowEnvBranchMapping {
		if len(sep.FindAllStringIndex(val, -1)) != 1 {
			return fmt.Errorf("Invalid GitflowEnvBranchMapping argument %s. Must be env:branch", val)
		}
	}

	if config.RequireExternalApproval && config.ApprovalURL == "" {
		return fmt.Errorf("--%s requires --%s to be set", RequireApprovalFlag, ApprovalURLFlag)
	}

	return nil
}

// setAtlantisURL sets the externally accessible URL for atlantis.
func setAtlantisURL(config *server.Config) error {
	if config.AtlantisURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("Failed to determine hostname: %v", err)
		}
		config.AtlantisURL = fmt.Sprintf("http://%s:%d", hostname, config.Port)
	}
	return nil
}

// setDataDir checks if ~ was used in data-dir and converts it to the actual
// home directory. If we don't do this, we'll create a directory called "~"
// instead of actually using home.
func setDataDir(config *server.Config) error {
	if strings.HasPrefix(config.DataDir, "~/") {
		expanded, err := homedir.Expand(config.DataDir)
		if err != nil {
			return errors.Wrap(err, "determining home directory")
		}
		config.DataDir = expanded
	}
	return nil
}

// trimAtSymbolFromUsers trims @ from the front of the github and gitlab usernames
func trimAtSymbolFromUsers(config *server.Config) {
	config.GithubUser = strings.TrimPrefix(config.GithubUser, "@")
	config.GitlabUser = strings.TrimPrefix(config.GitlabUser, "@")
}

// withErrPrint prints out any errors to a terminal in red.
func (s *ServerCmd) withErrPrint(f func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := f(cmd, args)
		if err != nil && !s.SilenceOutput {
			fmt.Fprintf(os.Stderr, "\033[31mError: %s\033[39m\n\n", err.Error())
		}
		return err
	}
}
