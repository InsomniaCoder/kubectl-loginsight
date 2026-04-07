package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/InsomniaCoder/kubectl-loginsight/internal/config"
	"github.com/InsomniaCoder/kubectl-loginsight/internal/llm"
	"github.com/InsomniaCoder/kubectl-loginsight/internal/logs"
)

var rootCmd = &cobra.Command{
	Use:   "kubectl-loginsight",
	Short: "Analyze Kubernetes logs using a local LLM",
	RunE:  run,
}

func Execute() {
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	model, _ := cmd.Flags().GetString("model")
	question, _ := cmd.Flags().GetString("question")
	baseURL, _ := cmd.Flags().GetString("base-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	filePath, _ := cmd.Flags().GetString("file")

	cfg := config.Load(model, baseURL, apiKey, maxTokens)

	if cfg.Model == "" {
		return fmt.Errorf("model is required: set --model or add 'model' to ~/.kube/log-insight.yaml")
	}

	var logSource *os.File
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("could not open file %s: %w", filePath, err)
		}
		defer f.Close()
		logSource = f
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("no log input: pipe logs via stdin or use --file")
		}
		logSource = os.Stdin
	}

	content, truncated, err := logs.Read(logSource, cfg.MaxTokens)
	if err != nil {
		return fmt.Errorf("reading logs: %w", err)
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "⚠  Logs truncated to %d tokens (oldest lines removed)\n", cfg.MaxTokens)
	}
	if content == "" {
		fmt.Fprintln(os.Stderr, "No log content found.")
		return nil
	}

	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	start := time.Now()
	answer, err := client.Ask(content, question)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	fmt.Println(answer)
	fmt.Fprintf(os.Stderr, "⏱  %.1fs\n", elapsed.Seconds())
	return nil
}

func init() {
	rootCmd.Flags().StringP("model", "m", "", "Model name (overrides config file)")
	rootCmd.Flags().StringP("question", "q", "", "Question to ask about the logs (optional, defaults to summarize)")
	rootCmd.Flags().String("base-url", "http://localhost:11434/v1", "OpenAI-compatible API base URL")
	rootCmd.Flags().String("api-key", "ollama", "API key")
	rootCmd.Flags().Int("max-tokens", 6500, "Max tokens of log content to send (model context size minus ~1700 headroom for prompt + response)")
	rootCmd.Flags().StringP("file", "f", "", "Read logs from file instead of stdin")
}
