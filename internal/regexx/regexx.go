package regexx

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"time"
)

type Match struct {
	Text   string   `json:"text"`
	Start  int      `json:"start"`
	End    int      `json:"end"`
	Groups []string `json:"groups"`
}

type Explanation struct {
	Pattern string   `json:"pattern"`
	Nodes   []string `json:"nodes"`
}

type DangerReport struct {
	Pattern string   `json:"pattern"`
	Risky   bool     `json:"risky"`
	Reasons []string `json:"reasons"`
}

type ExampleSet struct {
	Pattern     string   `json:"pattern"`
	Matching    []string `json:"matching"`
	NonMatching []string `json:"non_matching"`
}

type BenchmarkResult struct {
	Pattern  string `json:"pattern"`
	Matches  int    `json:"matches"`
	Duration string `json:"duration"`
}

func TestPattern(pattern string, input string) ([]Match, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	indexes := expression.FindAllStringSubmatchIndex(input, -1)
	matches := make([]Match, 0, len(indexes))
	for _, matchIndexes := range indexes {
		match := Match{
			Text:  input[matchIndexes[0]:matchIndexes[1]],
			Start: matchIndexes[0],
			End:   matchIndexes[1],
		}
		for i := 2; i < len(matchIndexes); i += 2 {
			if matchIndexes[i] == -1 {
				match.Groups = append(match.Groups, "")
				continue
			}
			match.Groups = append(match.Groups, input[matchIndexes[i]:matchIndexes[i+1]])
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func Find(pattern string, input string) ([]Match, error) {
	return TestPattern(pattern, input)
}

func Replace(pattern string, replacement string, input string) (string, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	return expression.ReplaceAllString(input, replacement), nil
}

func Escape(input string) string {
	return regexp.QuoteMeta(input)
}

func Unescape(input string) string {
	replacer := strings.NewReplacer(`\.`, ".", `\+`, "+", `\*`, "*", `\?`, "?", `\(`, "(", `\)`, ")", `\[`, "[", `\]`, "]", `\{`, "{", `\}`, "}", `\|`, "|", `\\`, `\`)
	return replacer.Replace(input)
}

func Explain(pattern string) (Explanation, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return Explanation{}, err
	}
	var nodes []string
	walk(parsed, 0, &nodes)
	return Explanation{Pattern: pattern, Nodes: nodes}, nil
}

func Visual(pattern string) (string, error) {
	explanation, err := Explain(pattern)
	if err != nil {
		return "", err
	}
	return strings.Join(explanation.Nodes, "\n") + "\n", nil
}

func Danger(pattern string) (DangerReport, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return DangerReport{}, err
	}
	report := DangerReport{Pattern: pattern}
	findRisk(parsed, false, &report)
	return report, nil
}

func Examples(pattern string) (ExampleSet, error) {
	if _, err := regexp.Compile(pattern); err != nil {
		return ExampleSet{}, err
	}
	return ExampleSet{
		Pattern:     pattern,
		Matching:    []string{"123", "user-42", "admin-7", "abc"},
		NonMatching: []string{"", "not a match", "###"},
	}, nil
}

func Benchmark(pattern string, input string) (BenchmarkResult, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return BenchmarkResult{}, err
	}
	start := time.Now()
	matches := expression.FindAllString(input, -1)
	return BenchmarkResult{Pattern: pattern, Matches: len(matches), Duration: time.Since(start).String()}, nil
}

func walk(node *syntax.Regexp, depth int, out *[]string) {
	*out = append(*out, strings.Repeat("  ", depth)+node.Op.String())
	for _, child := range node.Sub {
		walk(child, depth+1, out)
	}
}

func findRisk(node *syntax.Regexp, insideRepeat bool, report *DangerReport) {
	isRepeat := node.Op == syntax.OpStar || node.Op == syntax.OpPlus || node.Op == syntax.OpQuest || node.Op == syntax.OpRepeat
	if insideRepeat && isRepeat {
		report.Risky = true
		report.Reasons = append(report.Reasons, "nested repetition can cause excessive backtracking in some regex engines")
	}
	for _, child := range node.Sub {
		findRisk(child, insideRepeat || isRepeat, report)
	}
}
