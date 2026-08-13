package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/apiaudit"
)

type stringListFlag []string

func (values *stringListFlag) String() string { return strings.Join(*values, ",") }

func (values *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("case id must not be empty")
	}
	*values = append(*values, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, http.DefaultClient))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer, doer apiaudit.HTTPDoer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: api-audit <list|run> [options]")
		return 2
	}
	switch args[0] {
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		flags.SetOutput(stderr)
		suite := flags.String("suite", "", "case suite: openai-chat or seedance")
		casesRoot := flags.String("cases-root", filepath.FromSlash("tools/api-audit/cases"), "case definition root")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		cases, err := apiaudit.LoadSuite(*casesRoot, *suite)
		if err != nil {
			fmt.Fprintln(stderr, "CONFIG ERROR:", err)
			return 2
		}
		fmt.Fprintf(stdout, "%-8s %-10s %-12s %s\n", "ID", "DEFAULT", "DIMENSION", "NAME")
		for _, definition := range cases {
			fmt.Fprintf(stdout, "%-8s %-10t %-12s %s\n", definition.ID, definition.Default, definition.Dimension, definition.Name)
		}
		return 0

	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		flags.SetOutput(stderr)
		suite := flags.String("suite", "", "case suite: openai-chat or seedance")
		casesRoot := flags.String("cases-root", filepath.FromSlash("tools/api-audit/cases"), "case definition root")
		baseURL := flags.String("base-url", "", "HTTPS gateway base URL")
		model := flags.String("model", "", "model to audit")
		allCases := flags.Bool("all-cases", false, "run every case in the suite")
		allModels := flags.Bool("all-models", false, "run all configured Seedance models")
		dryRun := flags.Bool("dry-run", false, "render requests without network calls")
		confirmPaid := flags.Bool("confirm-paid-suite", false, "confirm a multi-task live Seedance run")
		noWait := flags.Bool("no-wait", false, "do not poll Seedance tasks to terminal status")
		keyEnv := flags.String("api-key-env", "API_AUDIT_API_KEY", "environment variable containing the bearer key")
		output := flags.String("output", "", "report output directory")
		pollInterval := flags.Duration("poll-interval", 10*time.Second, "Seedance polling interval")
		timeout := flags.Duration("timeout", 10*time.Minute, "per-case timeout")
		var caseIDs stringListFlag
		flags.Var(&caseIDs, "case", "case ID to run; repeat for multiple cases")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		if *suite != "openai-chat" && *suite != "seedance" {
			fmt.Fprintln(stderr, "CONFIG ERROR: --suite must be openai-chat or seedance")
			return 2
		}
		parsedBase, err := url.Parse(*baseURL)
		if err != nil || parsedBase.Scheme != "https" || parsedBase.Host == "" {
			fmt.Fprintln(stderr, "CONFIG ERROR: --base-url must be an absolute HTTPS URL")
			return 2
		}
		if *allModels && *suite != "seedance" {
			fmt.Fprintln(stderr, "CONFIG ERROR: --all-models is only valid for seedance")
			return 2
		}
		if *suite == "openai-chat" && strings.TrimSpace(*model) == "" {
			fmt.Fprintln(stderr, "CONFIG ERROR: --model is required for openai-chat")
			return 2
		}
		if *timeout <= 0 {
			fmt.Fprintln(stderr, "CONFIG ERROR: --timeout must be positive")
			return 2
		}
		if *pollInterval <= 0 {
			fmt.Fprintln(stderr, "CONFIG ERROR: --poll-interval must be positive")
			return 2
		}
		apiKey := strings.TrimSpace(getenv(*keyEnv))
		if !*dryRun && apiKey == "" {
			fmt.Fprintf(stderr, "CONFIG ERROR: environment variable %s is required for a live run\n", *keyEnv)
			return 2
		}
		cases, err := apiaudit.LoadSuite(*casesRoot, *suite)
		if err != nil {
			fmt.Fprintln(stderr, "CONFIG ERROR:", err)
			return 2
		}
		selected, err := apiaudit.SelectCases(cases, caseIDs, *allCases)
		if err != nil {
			fmt.Fprintln(stderr, "CONFIG ERROR:", err)
			return 2
		}
		config := apiaudit.RunConfig{
			Suite: *suite, BaseURL: *baseURL, APIKey: apiKey, Model: strings.TrimSpace(*model),
			DryRun: *dryRun, NoWait: *noWait, ConfirmPaidSuite: *confirmPaid,
			PollInterval: *pollInterval, Timeout: *timeout,
		}
		if *suite == "seedance" {
			if config.Model == "" {
				config.Model = apiaudit.DefaultSeedanceModel
			}
			if *allModels {
				config.Models = append([]string(nil), apiaudit.DefaultSeedanceModels...)
			}
		}
		runs, err := apiaudit.ExpandRuns(config, selected)
		if err != nil {
			fmt.Fprintln(stderr, "CONFIG ERROR:", err)
			return 2
		}
		fmt.Fprintf(stdout, "PLAN %d case run(s)\n", len(runs))
		for _, planned := range runs {
			fmt.Fprintf(stdout, "- %s %s [%s]\n", planned.ResultID, planned.Model, planned.Case.Kind)
		}

		results := make([]apiaudit.CaseResult, 0, len(runs))
		for _, planned := range runs {
			if config.DryRun && config.Suite == "openai-chat" {
				result := apiaudit.CaseResult{
					ID: planned.ResultID, Name: planned.Case.Name, Dimension: planned.Case.Dimension,
					Protocol: planned.Case.Protocol, Model: planned.Model, Status: apiaudit.StatusUnknown,
					Severity: planned.Case.Severity, Evidence: "dry-run: request was not submitted",
				}
				if planned.Case.Kind == "manual_unknown" {
					result.Evidence = "dry-run: manual/externally-instrumented case has no HTTP request"
				} else {
					body := make(map[string]any, len(planned.Case.Request.Body)+1)
					for key, value := range planned.Case.Request.Body {
						body[key] = value
					}
					if planned.Case.Kind != "models_contains" {
						body["model"] = planned.Model
					}
					if len(body) == 0 {
						body = nil
					}
					path := planned.Case.Request.Path
					if !strings.HasPrefix(path, "/") {
						path = "/" + path
					}
					result.Exchanges = []apiaudit.HTTPExchange{{Method: planned.Case.Request.Method, URL: strings.TrimRight(config.BaseURL, "/") + path, RequestBody: body}}
				}
				results = append(results, result)
				continue
			}
			caseContext, cancelCase := context.WithTimeout(context.Background(), config.Timeout)
			if config.Suite == "seedance" {
				results = append(results, apiaudit.RunSeedanceCase(caseContext, doer, config, planned))
			} else {
				caseConfig := config
				caseConfig.Model = planned.Model
				result := apiaudit.RunOpenAIChatCase(caseContext, doer, caseConfig, planned.Case)
				result.ID = planned.ResultID
				results = append(results, result)
			}
			cancelCase()
		}
		if *output == "" {
			*output = filepath.Join("output", "api-audit", time.Now().Format("20060102-150405"))
		}
		displayConfig := config
		if len(config.Models) > 1 {
			displayConfig.Model = strings.Join(config.Models, ", ")
		}
		report := apiaudit.BuildReport(displayConfig, results)
		if err := apiaudit.WriteReport(*output, report); err != nil {
			fmt.Fprintln(stderr, "REPORT ERROR:", err)
			return 1
		}
		fmt.Fprintf(stdout, "REPORT %s\n", filepath.Join(*output, "report.html"))
		fmt.Fprintf(stdout, "VERDICT %s\n", report.Verdict)
		if report.Summary.Fail > 0 {
			return 1
		}
		return 0
	default:
		fmt.Fprintln(stderr, "unknown command:", args[0])
		return 2
	}
}
