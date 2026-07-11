package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func NewArticle(root string, slug string, title string, module string, summary string) (string, error) {
	if err := validateSlug(slug); err != nil {
		return "", err
	}
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("title is required")
	}
	if strings.TrimSpace(module) == "" {
		module = "general"
	}
	if strings.TrimSpace(summary) == "" {
		summary = title
	}

	order := 1
	if catalog, err := LoadCatalog(root); err == nil {
		for _, article := range catalog.Articles {
			if article.Order >= order {
				order = article.Order + 1
			}
		}
	}

	dir := filepath.Join(root, "articles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, slug+".md")
	if exists(path) {
		return "", fmt.Errorf("article already exists: %s", path)
	}

	body := fmt.Sprintf(`---
id: %s
title: %s
module: %s
summary: %s
order: %d
---

# %s

Коротко объясни тему, покажи важные команды и добавь контекст для практики.
`, slug, title, module, summary, order, title)

	return path, os.WriteFile(path, []byte(body), 0o644)
}

func NewQuiz(root string, articleID string) (string, error) {
	if err := validateSlug(articleID); err != nil {
		return "", err
	}

	dir := filepath.Join(root, "quizzes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, articleID+".json")
	if exists(path) {
		return "", fmt.Errorf("quiz already exists: %s", path)
	}

	quiz := Quiz{
		ArticleID: articleID,
		Questions: []Question{
			{
				Text:        "Какой вариант правильный?",
				Options:     []string{"Первый вариант", "Второй вариант", "Третий вариант"},
				Answer:      0,
				Explanation: "Объясни, почему правильный ответ именно такой.",
			},
		},
	}

	return path, writeJSON(path, quiz)
}

func NewLab(root string, articleID string) (string, error) {
	if err := validateSlug(articleID); err != nil {
		return "", err
	}

	dir := filepath.Join(root, "labs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, articleID+".json")
	if exists(path) {
		return "", fmt.Errorf("lab already exists: %s", path)
	}

	lab := Lab{
		ArticleID: articleID,
		Title:     "Практика: " + articleID,
		Summary:   "Опиши, что пользователь должен сделать в консоли.",
		Environment: Environment{
			Image: "ubuntu:24.04",
		},
		Steps: []string{
			"Открой терминал лаборатории.",
			"Выполни команды из статьи.",
			"Запусти проверку результата.",
		},
		Checks: []Check{
			{
				Name:           "Example check",
				Kind:           "command",
				Command:        "true",
				ExpectExitCode: 0,
			},
		},
	}

	return path, writeJSON(path, lab)
}

func validateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug must match %s", slugPattern.String())
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
