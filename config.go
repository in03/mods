package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/adrg/xdg"
	"github.com/caarlos0/env/v8"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var help = map[string]string{
	"api":             "OpenAI compatible REST API (openai, localai).",
	"apis":            "Aliases and endpoints for OpenAI compatible REST API.",
	"model":           "Default model (gpt-3.5-turbo, gpt-4, ggml-gpt4all-j...).",
	"max-input-chars": "Default character limit on input to model.",
	"format":          "Ask for the response to be formatted as markdown (default).",
	"format-text":     "Text to append when using the -f flag.",
	"prompt":          "Include the prompt from the arguments and stdin, truncate stdin to specified number of lines.",
	"prompt-args":     "Include the prompt from the arguments in the response.",
	"quiet":           "Quiet mode (hide the spinner while loading).",
	"help":            "Show help and exit.",
	"version":         "Show version and exit.",
	"max-retries":     "Maximum number of times to retry API calls.",
	"no-limit":        "Turn off the client-side limit on the size of the input into the model.",
	"max-tokens":      "Maximum number of tokens in response.",
	"temp":            "Temperature (randomness) of results, from 0.0 to 2.0.",
	"topp":            "TopP, an alternative to temperature that narrows response, from 0.0 to 1.0.",
	"fanciness":       "Number of cycling characters in the 'generating' animation.",
	"status-text":     "Text to show while generating.",
	"settings":        "Open settings in your $EDITOR.",
	"reset-settings":  "Reset settings to the defaults, your old settings file will be backed up.",
}

// Model represents the LLM model used in the API call.
type Model struct {
	Name     string
	API      string
	MaxChars int      `yaml:"max-input-chars"`
	Aliases  []string `yaml:"aliases"`
	Fallback string   `yaml:"fallback"`
}

// API represents an API endpoint and its models.
type API struct {
	Name      string
	APIKeyEnv string           `yaml:"api-key-env"`
	BaseURL   string           `yaml:"base-url"`
	Models    map[string]Model `yaml:"models"`
}

// APIs is a type alias to allow custom YAML decoding.
type APIs []API

// UnmarshalYAML implements sorted API YAML decoding.
func (apis *APIs) UnmarshalYAML(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		var api API
		if err := node.Content[i+1].Decode(&api); err != nil {
			return fmt.Errorf("error decoding YAML file: %s", err)
		}
		api.Name = node.Content[i].Value
		*apis = append(*apis, api)
	}
	return nil
}

// Config holds the main configuration and is mapped to the YAML settings file.
type Config struct {
	Model             string  `yaml:"default-model" env:"MODEL"`
	Format            bool    `yaml:"format" env:"FORMAT"`
	Quiet             bool    `yaml:"quiet" env:"QUIET"`
	MaxTokens         int     `yaml:"max-tokens" env:"MAX_TOKENS"`
	MaxInputChars     int     `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`
	Temperature       float32 `yaml:"temp" env:"TEMP"`
	TopP              float32 `yaml:"topp" env:"TOPP"`
	NoLimit           bool    `yaml:"no-limit" env:"NO_LIMIT"`
	IncludePromptArgs bool    `yaml:"include-prompt-args" env:"INCLUDE_PROMPT_ARGS"`
	IncludePrompt     int     `yaml:"include-prompt" env:"INCLUDE_PROMPT"`
	MaxRetries        int     `yaml:"max-retries" env:"MAX_RETRIES"`
	Fanciness         uint    `yaml:"fanciness" env:"FANCINESS"`
	StatusText        string  `yaml:"status-text" env:"STATUS_TEXT"`
	FormatText        string  `yaml:"format-text" env:"FORMAT_TEXT"`
	APIs              APIs    `yaml:"apis"`
	API               string
	Models            map[string]Model
	ShowHelp          bool
	ResetSettings     bool
	Prefix            string
	Version           bool
	Settings          bool
	SettingsPath      string
}

func newConfig() (Config, error) {
	var c Config
	sp, err := xdg.ConfigFile(filepath.Join("mods", "mods.yml"))
	if err != nil {
		return c, fmt.Errorf("can't find settings path: %s", err)
	}
	c.SettingsPath = sp
	err = writeConfigFile(sp)
	if err != nil {
		return c, err
	}
	content, err := os.ReadFile(sp)
	if err != nil {
		return c, fmt.Errorf("can't read settings file: %s", err)
	}
	if err := yaml.Unmarshal(content, &c); err != nil {
		return c, fmt.Errorf("%s: %w", sp, err)
	}
	ms := make(map[string]Model)
	for _, api := range c.APIs {
		for mk, mv := range api.Models {
			mv.Name = mk
			mv.API = api.Name
			// only set the model key and aliases if they haven't already been used
			_, ok := ms[mk]
			if !ok {
				ms[mk] = mv
			}
			for _, a := range mv.Aliases {
				_, ok := ms[a]
				if !ok {
					ms[a] = mv
				}
			}
		}
	}
	c.Models = ms

	err = env.ParseWithOptions(&c, env.Options{Prefix: "MODS_"})
	if err != nil {
		return c, err
	}

	flag.StringVarP(&c.Model, "model", "m", c.Model, help["model"])
	flag.StringVarP(&c.API, "api", "a", c.API, help["api"])
	flag.BoolVarP(&c.Format, "format", "f", c.Format, help["format"])
	flag.IntVarP(&c.IncludePrompt, "prompt", "P", c.IncludePrompt, help["prompt"])
	flag.BoolVarP(&c.IncludePromptArgs, "prompt-args", "p", c.IncludePromptArgs, help["prompt-args"])
	flag.BoolVarP(&c.Quiet, "quiet", "q", c.Quiet, help["quiet"])
	flag.BoolVarP(&c.Settings, "settings", "s", false, help["settings"])
	flag.BoolVarP(&c.ShowHelp, "help", "h", false, help["help"])
	flag.BoolVarP(&c.Version, "version", "v", false, help["version"])
	flag.IntVar(&c.MaxRetries, "max-retries", c.MaxRetries, help["max-retries"])
	flag.BoolVar(&c.NoLimit, "no-limit", c.NoLimit, help["no-limit"])
	flag.IntVar(&c.MaxTokens, "max-tokens", c.MaxTokens, help["max-tokens"])
	flag.Float32Var(&c.Temperature, "temp", c.Temperature, help["temp"])
	flag.Float32Var(&c.TopP, "topp", c.TopP, help["topp"])
	flag.UintVar(&c.Fanciness, "fanciness", c.Fanciness, help["fanciness"])
	flag.StringVar(&c.StatusText, "status-text", c.StatusText, help["status-text"])
	flag.BoolVar(&c.ResetSettings, "reset-settings", c.ResetSettings, help["reset-settings"])
	flag.Lookup("prompt").NoOptDefVal = "-1"
	flag.Usage = usage
	flag.CommandLine.SortFlags = false
	flag.Parse()
	if c.Format && c.FormatText == "" {
		c.FormatText = "Format the response as markdown without enclosing backticks."
	}
	c.Prefix = strings.Join(flag.Args(), " ")

	return c, nil
}

func writeConfigFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		var c Config
		tmpl, err := template.New("config").Parse(strings.TrimSpace(configTemplate))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}

		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		m := struct {
			Config Config
			Help   map[string]string
		}{
			Config: c,
			Help:   help,
		}
		if err := tmpl.Execute(f, m); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func usage() {
	r := lipgloss.DefaultRenderer()
	s := makeStyles(r)
	appName := filepath.Base(os.Args[0])

	if r.ColorProfile() == termenv.TrueColor {
		appName = makeGradientText(s.appName, appName)
	}

	fmt.Printf("GPT on the command line. Built for pipelines.\n\n")
	fmt.Printf(
		"Usage:\n  %s %s\n\n",
		appName,
		s.cliArgs.Render("[OPTIONS] [PREFIX TERM]"),
	)
	fmt.Println("Options:")
	flag.VisitAll(func(f *flag.Flag) {
		if f.Shorthand == "" {
			fmt.Printf(
				"  %-42s %s\n",
				s.flag.Render("--"+f.Name),
				s.flagDesc.Render(f.Usage),
			)
		} else {
			fmt.Printf(
				"  %s%s %-38s %s\n",
				s.flag.Render("-"+f.Shorthand),
				s.flagComma,
				s.flag.Render("--"+f.Name),
				s.flagDesc.Render(f.Usage),
			)
		}
	})
	desc, example := randomExample()
	fmt.Printf(
		"\nExample:\n  %s\n  %s\n",
		s.comment.Render("# "+desc),
		cheapHighlighting(s, example),
	)
}
