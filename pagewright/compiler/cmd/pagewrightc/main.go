package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/compile"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse command line flags
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)
	themeDir := buildCmd.String("theme", "", "Theme directory (required)")
	contentDir := buildCmd.String("content", "", "Content directory (required)")
	outputDir := buildCmd.String("out", "dist", "Output directory")
	baseURL := buildCmd.String("base-url", "", "Base URL for the site (e.g., https://example.com)")

	// Check for subcommand
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}

	switch os.Args[1] {
	case "build":
		buildCmd.Parse(os.Args[2:])
		return runBuild(*themeDir, *contentDir, *outputDir, *baseURL)

	case "version", "-v", "--version":
		fmt.Printf("pagewrightc version %s\n", version)
		return nil

	case "help", "-h", "--help":
		printUsage()
		return nil

	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

func runBuild(themeDir, contentDir, outputDir, baseURL string) error {
	// Validate required arguments
	if themeDir == "" {
		return fmt.Errorf("--theme is required")
	}
	if contentDir == "" {
		return fmt.Errorf("--content is required")
	}
	if baseURL == "" {
		baseURL = "" // default to empty, will be used as relative paths
	}

	fmt.Println("Pagewright Compiler v" + version)
	fmt.Println("=====================================")
	fmt.Printf("Theme:   %s\n", themeDir)
	fmt.Printf("Content: %s\n", contentDir)
	fmt.Printf("Output:  %s\n", outputDir)
	if baseURL != "" {
		fmt.Printf("BaseURL: %s\n", baseURL)
	}
	fmt.Println()

	// Load configuration
	cfg, err := config.Load(themeDir, contentDir, outputDir, baseURL)
	if err != nil {
		return err
	}

	// Create and run pipeline
	pipeline, err := compile.NewPipeline(cfg)
	if err != nil {
		return err
	}

	return pipeline.Run()
}

func printUsage() {
	fmt.Println("Pagewright Compiler - Static site generator")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  pagewrightc build --theme <dir> --content <dir> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  build       Compile a static website")
	fmt.Println("  version     Show version information")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("Build Options:")
	fmt.Println("  --theme <dir>      Theme directory (required)")
	fmt.Println("  --content <dir>    Content directory (required)")
	fmt.Println("  --out <dir>        Output directory (default: dist)")
	fmt.Println("  --base-url <url>   Base URL for the site")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  pagewrightc build \\")
	fmt.Println("    --theme ./pagewright/themes/starter \\")
	fmt.Println("    --content ./my-site/content \\")
	fmt.Println("    --out ./my-site/dist \\")
	fmt.Println("    --base-url https://example.com")
}
