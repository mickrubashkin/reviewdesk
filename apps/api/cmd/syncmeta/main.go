package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	sequence = "onboarding"
)

type seedEmailMeta struct {
	Title      string `json:"title"`
	Subject    string `json:"subject"`
	Preheader  string `json:"preheader"`
	SendTiming string `json:"send_timing"`
}

type seedEmailFile struct {
	Key      string
	Path     string
	Name     string
	Language string
	Variant  string
}

var (
	titlePattern       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	preheaderPattern   = regexp.MustCompile(`(?is)<span\b[^>]*class=["'][^"']*preheader[^"']*["'][^>]*>(.*?)</span>`)
	bodyPattern        = regexp.MustCompile(`(?is)<body\b[^>]*>(.*?)</body>`)
	firstDivPattern    = regexp.MustCompile(`(?is)<div\b[^>]*>(.*?)</div>`)
	htmlTagPattern     = regexp.MustCompile(`(?is)<[^>]*>`)
	whitespacePattern  = regexp.MustCompile(`\s+`)
	zwnjPaddingPattern = regexp.MustCompile(`(?i)(&zwnj;|&nbsp;)+`)
	numericEntity      = regexp.MustCompile(`&#(x[0-9a-fA-F]+|[0-9]+);`)
	namedEntities      = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
		"&nbsp;", " ",
		"&zwnj;", " ",
		"&ndash;", "-",
		"&mdash;", "-",
	)
)

func main() {
	seedDir, err := findSeedDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find seed emails directory: %v\n", err)
		os.Exit(1)
	}

	emails, err := loadSeedEmailFiles(seedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to load seed emails: %v\n", err)
		os.Exit(1)
	}

	currentMeta, err := loadCurrentMeta(seedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to load current meta: %v\n", err)
		os.Exit(1)
	}

	nextMeta := map[string]seedEmailMeta{}
	for _, email := range emails {
		meta, err := buildMeta(email, currentMetaForEmail(currentMeta, email))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to build meta for %s: %v\n", email.Path, err)
			os.Exit(1)
		}

		nextMeta[email.Key] = meta
	}

	if err := writeMeta(filepath.Join(seedDir, "meta.json"), nextMeta); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to write meta: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Synced %d email meta entries\n", len(nextMeta))
}

func findSeedDir() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "db", "seeds", "emails")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", fmt.Errorf("db/seeds/emails was not found from %s or its parents", workingDir)
}

func loadSeedEmailFiles(root string) ([]seedEmailFile, error) {
	emails := []seedEmailFile{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		parts := strings.Split(filepath.ToSlash(relativePath), "/")
		if len(parts) != 3 {
			return fmt.Errorf("expected {stage}/{email}/{language}.html, got %s", relativePath)
		}

		_, stage := parseOrderedName(parts[0])
		_, emailName := parseOrderedName(parts[1])
		fileName := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))
		language, variant := parseLanguageVariant(fileName)
		key := metaKey(stage, emailName, language, variant)

		emails = append(emails, seedEmailFile{
			Key:      key,
			Path:     path,
			Name:     emailName,
			Language: language,
			Variant:  variant,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(emails, func(i, j int) bool {
		return emails[i].Key < emails[j].Key
	})

	return emails, nil
}

func parseLanguageVariant(fileName string) (string, string) {
	if fileName == "old" {
		return "en", "old"
	}

	if language, ok := strings.CutSuffix(fileName, "-old"); ok {
		return language, "old"
	}

	return fileName, "new"
}

func metaKey(stage string, emailName string, language string, variant string) string {
	keyLanguage := language
	if variant == "old" {
		keyLanguage = language + "-old"
	}

	return strings.Join([]string{sequence, stage, emailName, keyLanguage}, "/")
}

func currentMetaForEmail(currentMeta map[string]seedEmailMeta, email seedEmailFile) seedEmailMeta {
	if meta, ok := currentMeta[email.Key]; ok {
		return meta
	}

	if email.Variant == "old" && email.Language == "en" {
		legacyOldKey := strings.Join([]string{sequence, stageFromKey(email.Key), email.Name, "old"}, "/")
		if meta, ok := currentMeta[legacyOldKey]; ok {
			return meta
		}
	}

	legacyLanguageKey := strings.Join([]string{sequence, stageFromKey(email.Key), email.Name, email.Language}, "/")
	if meta, ok := currentMeta[legacyLanguageKey]; ok {
		return meta
	}

	return seedEmailMeta{}
}

func stageFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) >= 2 {
		return parts[1]
	}

	return ""
}

func loadCurrentMeta(root string) (map[string]seedEmailMeta, error) {
	metaPath := filepath.Join(root, "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]seedEmailMeta{}, nil
		}

		return nil, err
	}

	meta := map[string]seedEmailMeta{}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}

	return meta, nil
}

func buildMeta(email seedEmailFile, current seedEmailMeta) (seedEmailMeta, error) {
	htmlBytes, err := os.ReadFile(email.Path)
	if err != nil {
		return seedEmailMeta{}, err
	}

	title := current.Title
	if title == "" {
		title = titleFromName(email.Name)
	}

	sendTiming := current.SendTiming
	if sendTiming == "" {
		sendTiming = sendTimingFromName(email.Name)
	}

	htmlText := string(htmlBytes)

	return seedEmailMeta{
		Title:      title,
		Subject:    extractSubject(htmlText),
		Preheader:  extractPreheader(htmlText),
		SendTiming: sendTiming,
	}, nil
}

func writeMeta(path string, meta map[string]seedEmailMeta) error {
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buffer bytes.Buffer
	buffer.WriteString("{\n")
	for index, key := range keys {
		metaBytes, err := json.MarshalIndent(meta[key], "  ", "  ")
		if err != nil {
			return err
		}

		buffer.WriteString(fmt.Sprintf("  %q: ", key))
		buffer.Write(metaBytes)
		if index < len(keys)-1 {
			buffer.WriteString(",")
		}
		buffer.WriteString("\n")
	}
	buffer.WriteString("}\n")

	return os.WriteFile(path, buffer.Bytes(), 0644)
}

func extractSubject(htmlText string) string {
	matches := titlePattern.FindStringSubmatch(htmlText)
	if len(matches) < 2 {
		return ""
	}

	return cleanHTMLText(matches[1])
}

func extractPreheader(htmlText string) string {
	matches := preheaderPattern.FindStringSubmatch(htmlText)
	if len(matches) >= 2 {
		return cleanHTMLText(matches[1])
	}

	bodyMatches := bodyPattern.FindStringSubmatch(htmlText)
	body := htmlText
	if len(bodyMatches) >= 2 {
		body = bodyMatches[1]
	}

	divMatches := firstDivPattern.FindStringSubmatch(body)
	if len(divMatches) < 2 {
		return ""
	}

	return cleanHTMLText(divMatches[1])
}

func cleanHTMLText(value string) string {
	value = zwnjPaddingPattern.ReplaceAllString(value, " ")
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = decodeHTMLEntities(value)
	value = strings.ReplaceAll(value, "\u200c", " ")
	value = whitespacePattern.ReplaceAllString(value, " ")

	return strings.TrimSpace(value)
}

func decodeHTMLEntities(value string) string {
	value = namedEntities.Replace(value)

	return numericEntity.ReplaceAllStringFunc(value, func(entity string) string {
		matches := numericEntity.FindStringSubmatch(entity)
		if len(matches) < 2 {
			return entity
		}

		raw := matches[1]
		base := 10
		if strings.HasPrefix(raw, "x") || strings.HasPrefix(raw, "X") {
			base = 16
			raw = raw[1:]
		}

		codepoint, err := strconv.ParseInt(raw, base, 32)
		if err != nil {
			return entity
		}

		return string(rune(codepoint))
	})
}

func sendTimingFromName(name string) string {
	switch name {
	case "follow-up-1":
		return "2 days after previous email"
	case "follow-up-2":
		return "5 days after previous email"
	case "last-call":
		return "14 days after previous email"
	default:
		return "immediately"
	}
}

func parseOrderedName(name string) (int, string) {
	for i, char := range name {
		if !unicode.IsDigit(char) {
			if i > 0 && (char == '-' || char == '_') {
				order, err := strconv.Atoi(name[:i])
				if err == nil {
					return order, name[i+1:]
				}
			}

			return 0, name
		}
	}

	return 0, name
}

func titleFromName(name string) string {
	words := strings.FieldsFunc(name, func(char rune) bool {
		return char == '-' || char == '_'
	})

	for i, word := range words {
		words[i] = capitalize(word)
	}

	return strings.Join(words, " ")
}

func capitalize(word string) string {
	if word == "" {
		return word
	}

	runes := []rune(word)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
