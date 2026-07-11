package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/soul4bit/rootops/internal/config"
	"github.com/soul4bit/rootops/internal/content"
	"github.com/soul4bit/rootops/internal/web"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		args = []string{"serve"}
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "content":
		return runContent(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func runServe(args []string, stdout io.Writer, stderr io.Writer) int {
	cfg := config.Load()

	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP address")
	flags.StringVar(&cfg.ContentDir, "content", cfg.ContentDir, "content directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	server, err := web.NewServer(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "server init failed: %v\n", err)
		return 1
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Fprintf(stdout, "RootOPS listening on http://%s\n", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "server failed: %v\n", err)
		return 1
	}

	_ = httpServer.Shutdown(context.Background())
	return 0
}

func runContent(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printContentHelp(stderr)
		return 2
	}

	cfg := config.Load()

	switch args[0] {
	case "list":
		return contentList(cfg, stdout, stderr)
	case "validate":
		return contentValidate(cfg, stdout, stderr)
	case "new":
		return contentNew(cfg, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown content command: %s\n\n", args[0])
		printContentHelp(stderr)
		return 2
	}
}

func contentList(cfg config.Config, stdout io.Writer, stderr io.Writer) int {
	catalog, err := content.LoadCatalog(cfg.ContentDir)
	if err != nil {
		fmt.Fprintf(stderr, "content load failed: %v\n", err)
		return 1
	}

	if len(catalog.Articles) == 0 {
		fmt.Fprintln(stdout, "No articles yet.")
		return 0
	}

	for _, article := range catalog.Articles {
		hasQuiz := "no"
		hasLab := "no"
		if _, ok := catalog.Quizzes[article.ID]; ok {
			hasQuiz = "yes"
		}
		if _, ok := catalog.Labs[article.ID]; ok {
			hasLab = "yes"
		}
		fmt.Fprintf(stdout, "%02d  %-24s  module=%-12s quiz=%s lab=%s  %s\n", article.Order, article.ID, article.Module, hasQuiz, hasLab, article.Title)
	}
	return 0
}

func contentValidate(cfg config.Config, stdout io.Writer, stderr io.Writer) int {
	catalog, err := content.LoadCatalog(cfg.ContentDir)
	if err != nil {
		fmt.Fprintf(stderr, "content load failed: %v\n", err)
		return 1
	}

	problems := content.ValidateCatalog(catalog)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(stderr, "- %v\n", problem)
		}
		return 1
	}

	fmt.Fprintf(stdout, "Content OK: %d articles, %d quizzes, %d labs\n", len(catalog.Articles), len(catalog.Quizzes), len(catalog.Labs))
	return 0
}

func contentNew(cfg config.Config, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 2 {
		printContentHelp(stderr)
		return 2
	}

	kind := args[0]
	slug := args[1]

	switch kind {
	case "article":
		flags := flag.NewFlagSet("content new article", flag.ContinueOnError)
		flags.SetOutput(stderr)
		title := flags.String("title", "", "article title")
		module := flags.String("module", "general", "article module")
		summary := flags.String("summary", "", "article summary")
		if err := flags.Parse(args[2:]); err != nil {
			return 2
		}
		path, err := content.NewArticle(cfg.ContentDir, slug, *title, *module, *summary)
		return reportCreated(path, err, stdout, stderr)
	case "quiz":
		path, err := content.NewQuiz(cfg.ContentDir, slug)
		return reportCreated(path, err, stdout, stderr)
	case "lab":
		path, err := content.NewLab(cfg.ContentDir, slug)
		return reportCreated(path, err, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown content kind: %s\n\n", kind)
		printContentHelp(stderr)
		return 2
	}
}

func reportCreated(path string, err error, stdout io.Writer, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "create failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "created %s\n", path)
	return 0
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `RootOPS

Commands:
  serve                         Start web preview
  content list                  List articles, quizzes and labs
  content validate              Validate content catalog
  content new article <slug>    Create article skeleton
  content new quiz <slug>       Create quiz skeleton
  content new lab <slug>        Create console lab skeleton`)
}

func printContentHelp(out io.Writer) {
	fmt.Fprintln(out, `Content commands:
  content list
  content validate
  content new article <slug> --title "Title" --module linux --summary "Short text"
  content new quiz <slug>
  content new lab <slug>`)
}
