package content

import "fmt"

func ValidateCatalog(catalog *Catalog) []error {
	if catalog == nil {
		return []error{fmt.Errorf("catalog is nil")}
	}

	var problems []error
	articleIDs := make(map[string]bool, len(catalog.Articles))

	for _, article := range catalog.Articles {
		if articleIDs[article.ID] {
			problems = append(problems, fmt.Errorf("duplicate article id %q", article.ID))
		}
		articleIDs[article.ID] = true
	}

	for articleID, quiz := range catalog.Quizzes {
		if !articleIDs[articleID] {
			problems = append(problems, fmt.Errorf("quiz references missing article %q", articleID))
		}
		for index, question := range quiz.Questions {
			if question.Text == "" {
				problems = append(problems, fmt.Errorf("quiz %q question %d has empty text", articleID, index+1))
			}
			if len(question.Options) < 2 {
				problems = append(problems, fmt.Errorf("quiz %q question %d needs at least two options", articleID, index+1))
			}
			if question.Answer < 0 || question.Answer >= len(question.Options) {
				problems = append(problems, fmt.Errorf("quiz %q question %d has invalid answer index", articleID, index+1))
			}
		}
	}

	for articleID, lab := range catalog.Labs {
		if !articleIDs[articleID] {
			problems = append(problems, fmt.Errorf("lab references missing article %q", articleID))
		}
		if lab.Title == "" {
			problems = append(problems, fmt.Errorf("lab %q has empty title", articleID))
		}
		if lab.Environment.Image == "" {
			problems = append(problems, fmt.Errorf("lab %q has empty environment image", articleID))
		}
		if len(lab.Checks) == 0 {
			problems = append(problems, fmt.Errorf("lab %q has no checks", articleID))
		}
		for index, check := range lab.Checks {
			if check.Name == "" {
				problems = append(problems, fmt.Errorf("lab %q check %d has empty name", articleID, index+1))
			}
			if check.Kind == "" {
				problems = append(problems, fmt.Errorf("lab %q check %d has empty kind", articleID, index+1))
			}
			if check.Command == "" {
				problems = append(problems, fmt.Errorf("lab %q check %d has empty command", articleID, index+1))
			}
		}
	}

	return problems
}
