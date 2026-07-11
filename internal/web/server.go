package web

import (
	"embed"
	"html"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/soul4bit/rootops/internal/config"
	"github.com/soul4bit/rootops/internal/content"
)

//go:embed templates/*.html
var templateFiles embed.FS

type Server struct {
	cfg       config.Config
	templates *template.Template
}

type homePage struct {
	Articles []articlePreview
}

type articlePreview struct {
	Article content.Article
	HasQuiz bool
	HasLab  bool
}

type articlePage struct {
	Article content.Article
	Quiz    content.Quiz
	Lab     content.Lab
	HasQuiz bool
	HasLab  bool
	Body    template.HTML
}

func NewServer(cfg config.Config) (*Server, error) {
	templates, err := template.ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, templates: templates}, nil
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.handleHome)
	mux.HandleFunc("GET /articles/{id}", server.handleArticle)
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(server.cfg.ProjectDir, "assets")))))
	return securityHeaders(mux)
}

func (server *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	catalog, err := content.LoadCatalog(server.cfg.ContentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	previews := make([]articlePreview, 0, len(catalog.Articles))
	for _, article := range catalog.Articles {
		_, hasQuiz := catalog.Quizzes[article.ID]
		_, hasLab := catalog.Labs[article.ID]
		previews = append(previews, articlePreview{Article: article, HasQuiz: hasQuiz, HasLab: hasLab})
	}

	render(w, server.templates, "index.html", homePage{Articles: previews})
}

func (server *Server) handleArticle(w http.ResponseWriter, r *http.Request) {
	catalog, err := content.LoadCatalog(server.cfg.ContentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	article, ok := catalog.ArticleByID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	quiz, hasQuiz := catalog.Quizzes[article.ID]
	lab, hasLab := catalog.Labs[article.ID]
	render(w, server.templates, "article.html", articlePage{
		Article: article,
		Quiz:    quiz,
		Lab:     lab,
		HasQuiz: hasQuiz,
		HasLab:  hasLab,
		Body:    markdownToHTML(article.Body),
	})
}

func (server *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func render(w http.ResponseWriter, templates *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func markdownToHTML(markdown string) template.HTML {
	var out strings.Builder
	inList := false
	inCode := false

	closeList := func() {
		if inList {
			out.WriteString("</ul>")
			inList = false
		}
	}

	for _, rawLine := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "```") {
			closeList()
			if inCode {
				out.WriteString("</code></pre>")
			} else {
				out.WriteString("<pre><code>")
			}
			inCode = !inCode
			continue
		}

		if inCode {
			out.WriteString(html.EscapeString(rawLine))
			out.WriteByte('\n')
			continue
		}

		if line == "" {
			closeList()
			continue
		}

		switch {
		case strings.HasPrefix(line, "### "):
			closeList()
			out.WriteString("<h3>" + html.EscapeString(strings.TrimPrefix(line, "### ")) + "</h3>")
		case strings.HasPrefix(line, "## "):
			closeList()
			out.WriteString("<h2>" + html.EscapeString(strings.TrimPrefix(line, "## ")) + "</h2>")
		case strings.HasPrefix(line, "# "):
			closeList()
			out.WriteString("<h1>" + html.EscapeString(strings.TrimPrefix(line, "# ")) + "</h1>")
		case strings.HasPrefix(line, "- "):
			if !inList {
				out.WriteString("<ul>")
				inList = true
			}
			out.WriteString("<li>" + html.EscapeString(strings.TrimPrefix(line, "- ")) + "</li>")
		default:
			closeList()
			out.WriteString("<p>" + html.EscapeString(line) + "</p>")
		}
	}
	closeList()
	if inCode {
		out.WriteString("</code></pre>")
	}

	return template.HTML(out.String())
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
