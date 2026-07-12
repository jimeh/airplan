package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type configShowOptions struct {
	config rootOptions
	json   bool
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect airplan configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "Print the config file JSON Schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := airplan.ConfigSchema()
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	})
	cmd.AddCommand(newConfigShowCmd())

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	opts := &configShowOptions{}
	cmd := &cobra.Command{
		Use:           "show",
		Short:         "Show resolved configuration and its sources",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd, opts)
		},
	}
	addConfigResolutionFlags(cmd.Flags(), &opts.config)
	cmd.Flags().BoolVarP(&opts.json, "json", "j", false,
		"print structured JSON instead of a table")
	return cmd
}

func runConfigShow(cmd *cobra.Command, opts *configShowOptions) error {
	resolution, err := airplan.ResolveConfig(airplan.ConfigOptions{
		Path:      opts.config.config,
		Profile:   opts.config.profile,
		Overrides: flagOverrides(cmd, &opts.config),
	})
	if err != nil {
		return err
	}
	for _, warning := range resolution.Config.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(
			configShowJSONOutput(resolution),
		)
	}
	return printConfigShow(cmd.OutOrStdout(), resolution)
}

type configDisplayField struct {
	Name      string
	Value     any
	Set       bool
	Sensitive bool
	Source    *airplan.ConfigSource
}

func configDisplayFields(
	resolution *airplan.ConfigResolution,
) []configDisplayField {
	cfg := resolution.Config
	fields := []configDisplayField{
		{Name: "endpoint", Value: cfg.Endpoint, Set: cfg.Endpoint != ""},
		{Name: "bucket", Value: cfg.Bucket, Set: cfg.Bucket != ""},
		{Name: "region", Value: cfg.Region, Set: cfg.Region != ""},
		{
			Name: "access_key_id", Set: cfg.AccessKeyID != "",
			Sensitive: true,
		},
		{
			Name: "secret_access_key", Set: cfg.SecretAccessKey != "",
			Sensitive: true,
		},
		{
			Name: "public_base_url", Value: cfg.PublicBaseURL,
			Set: cfg.PublicBaseURL != "",
		},
		{Name: "key_prefix", Value: cfg.KeyPrefix, Set: cfg.KeyPrefix != ""},
		{Name: "template", Value: cfg.Template, Set: cfg.Template != ""},
		{Name: "no_source", Value: cfg.NoSource, Set: true},
		{Name: "indexable", Value: cfg.Indexable, Set: true},
		{
			Name: "no_external_assets", Value: cfg.NoExternalAssets,
			Set: true,
		},
		{
			Name: "mermaid_url", Value: cfg.MermaidURL,
			Set: cfg.MermaidURL != "",
		},
		{Name: "repo", Value: cfg.Repository, Set: cfg.Repository != ""},
		{Name: "timeout", Value: cfg.Timeout.String(), Set: true},
	}
	for i := range fields {
		fields[i].Source = resolution.Fields[fields[i].Name].Source
	}
	return fields
}

func printConfigShow(
	w io.Writer,
	resolution *airplan.ConfigResolution,
) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	configPath := resolution.ConfigPath.Path
	if !resolution.ConfigPath.Exists {
		configPath += " (not found)"
	}
	if _, err := fmt.Fprintf(tw, "CONFIG FILE\t%s\t%s\n",
		configPath,
		formatConfigSource(&resolution.ConfigPath.Source)); err != nil {
		return err
	}
	profile := resolution.Profile.Name
	if profile == "" {
		profile = "<root>"
	}
	if _, err := fmt.Fprintf(tw, "PROFILE\t%s\t%s\n",
		profile, formatConfigSource(&resolution.Profile.Source)); err != nil {
		return err
	}
	credentialMode := configCredentialMode(resolution.Config)
	switch credentialMode {
	case "standard_aws_chain":
		credentialMode = "standard AWS credential chain"
	case "partial":
		credentialMode = "partial explicit configuration"
	}
	if _, err := fmt.Fprintf(tw, "CREDENTIALS\t%s\n\n", credentialMode); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "FIELD\tVALUE\tSOURCE"); err != nil {
		return err
	}
	for _, field := range configDisplayFields(resolution) {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n",
			field.Name, configTableValue(field),
			formatConfigSource(field.Source)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func configTableValue(field configDisplayField) string {
	if !field.Set {
		return "<unset>"
	}
	if field.Sensitive {
		return "<set>"
	}
	switch value := field.Value.(type) {
	case bool:
		return strconv.FormatBool(value)
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func formatConfigSource(source *airplan.ConfigSource) string {
	if source == nil {
		return "<unset>"
	}
	return source.Name
}

func configCredentialMode(cfg *airplan.Config) string {
	switch {
	case cfg.AccessKeyID == "" && cfg.SecretAccessKey == "":
		return "standard_aws_chain"
	case cfg.AccessKeyID == "" || cfg.SecretAccessKey == "":
		return "partial"
	default:
		return "explicit"
	}
}

type configShowJSON struct {
	ConfigFile     airplan.ConfigPathResolution   `json:"config_file"`
	Profile        configShowJSONProfile          `json:"profile"`
	CredentialMode string                         `json:"credential_mode"`
	Fields         map[string]configShowJSONField `json:"fields"`
}

type configShowJSONProfile struct {
	Name   *string              `json:"name"`
	Root   bool                 `json:"root"`
	Source airplan.ConfigSource `json:"source"`
}

type configShowJSONField struct {
	Value     any                   `json:"value"`
	Set       bool                  `json:"set"`
	Sensitive bool                  `json:"sensitive"`
	Source    *airplan.ConfigSource `json:"source"`
}

func configShowJSONOutput(resolution *airplan.ConfigResolution) configShowJSON {
	profile := configShowJSONProfile{
		Root: resolution.Profile.Name == "", Source: resolution.Profile.Source,
	}
	if resolution.Profile.Name != "" {
		name := resolution.Profile.Name
		profile.Name = &name
	}
	credentialMode := configCredentialMode(resolution.Config)
	fields := make(map[string]configShowJSONField)
	for _, field := range configDisplayFields(resolution) {
		value := field.Value
		if field.Sensitive || !field.Set {
			value = nil
		}
		fields[field.Name] = configShowJSONField{
			Value: value, Set: field.Set, Sensitive: field.Sensitive,
			Source: field.Source,
		}
	}
	return configShowJSON{
		ConfigFile: resolution.ConfigPath,
		Profile:    profile, CredentialMode: credentialMode, Fields: fields,
	}
}
