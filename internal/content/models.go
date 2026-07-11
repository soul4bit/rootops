package content

type Catalog struct {
	Articles []Article
	Quizzes  map[string]Quiz
	Labs     map[string]Lab
}

type Article struct {
	ID      string
	Title   string
	Module  string
	Summary string
	Order   int
	Body    string
	Path    string
}

type Quiz struct {
	ArticleID string     `json:"article_id"`
	Questions []Question `json:"questions"`
	Path      string     `json:"-"`
}

type Question struct {
	Text        string   `json:"text"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
}

type Lab struct {
	ArticleID   string      `json:"article_id"`
	Title       string      `json:"title"`
	Summary     string      `json:"summary"`
	Environment Environment `json:"environment"`
	Steps       []string    `json:"steps"`
	Checks      []Check     `json:"checks"`
	Path        string      `json:"-"`
}

type Environment struct {
	Image   string   `json:"image"`
	Command []string `json:"command,omitempty"`
}

type Check struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Command        string `json:"command"`
	Expect         string `json:"expect,omitempty"`
	ExpectExitCode int    `json:"expect_exit_code"`
}
