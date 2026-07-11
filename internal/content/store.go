package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func LoadCatalog(root string) (*Catalog, error) {
	catalog := &Catalog{
		Quizzes: make(map[string]Quiz),
		Labs:    make(map[string]Lab),
	}

	articles, err := loadArticles(filepath.Join(root, "articles"))
	if err != nil {
		return nil, err
	}
	catalog.Articles = articles

	quizzes, err := loadQuizzes(filepath.Join(root, "quizzes"))
	if err != nil {
		return nil, err
	}
	catalog.Quizzes = quizzes

	labs, err := loadLabs(filepath.Join(root, "labs"))
	if err != nil {
		return nil, err
	}
	catalog.Labs = labs

	return catalog, nil
}

func (catalog *Catalog) ArticleByID(id string) (Article, bool) {
	for _, article := range catalog.Articles {
		if article.ID == id {
			return article, true
		}
	}
	return Article{}, false
}

func loadArticles(dir string) ([]Article, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	articles := make([]Article, 0, len(files))
	for _, path := range files {
		article, err := readArticle(path)
		if err != nil {
			return nil, err
		}
		articles = append(articles, article)
	}

	sort.SliceStable(articles, func(i, j int) bool {
		if articles[i].Order == articles[j].Order {
			return articles[i].ID < articles[j].ID
		}
		return articles[i].Order < articles[j].Order
	})

	return articles, nil
}

func readArticle(path string) (Article, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Article{}, err
	}

	meta, body, err := splitFrontMatter(string(raw))
	if err != nil {
		return Article{}, fmt.Errorf("%s: %w", path, err)
	}

	order, _ := strconv.Atoi(meta["order"])
	article := Article{
		ID:      meta["id"],
		Title:   meta["title"],
		Module:  meta["module"],
		Summary: meta["summary"],
		Order:   order,
		Body:    strings.TrimSpace(body),
		Path:    path,
	}

	if article.ID == "" {
		return Article{}, fmt.Errorf("%s: missing id", path)
	}
	if article.Title == "" {
		return Article{}, fmt.Errorf("%s: missing title", path)
	}
	if article.Module == "" {
		article.Module = "general"
	}

	return article, nil
}

func splitFrontMatter(raw string) (map[string]string, string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return nil, "", errors.New("missing front matter")
	}

	rest := strings.TrimPrefix(raw, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, "", errors.New("front matter is not closed")
	}

	meta := make(map[string]string)
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("bad front matter line %q", line)
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}

	return meta, rest[end+len("\n---\n"):], nil
}

func loadQuizzes(dir string) (map[string]Quiz, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}

	quizzes := make(map[string]Quiz, len(files))
	for _, path := range files {
		var quiz Quiz
		if err := readJSON(path, &quiz); err != nil {
			return nil, err
		}
		quiz.Path = path
		if quiz.ArticleID != "" {
			quizzes[quiz.ArticleID] = quiz
		}
	}
	return quizzes, nil
}

func loadLabs(dir string) (map[string]Lab, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}

	labs := make(map[string]Lab, len(files))
	for _, path := range files {
		var lab Lab
		if err := readJSON(path, &lab); err != nil {
			return nil, err
		}
		lab.Path = path
		if lab.ArticleID != "" {
			labs[lab.ArticleID] = lab
		}
	}
	return labs, nil
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
