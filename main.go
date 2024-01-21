package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type ADRMeta struct {
	Index   int
	Authors []string
	Date    time.Time
	Status  string
	Tags    []string
	Path    string
}

type ADR struct {
	Heading string
	Meta    ADRMeta
}

var (
	validStatus = []string{"Approved", "Partially Implemented", "Implemented"}
)

func parseCommaList(l string) []string {
	tags := strings.Split(l, ",")
	res := []string{}
	for _, t := range tags {
		res = append(res, strings.TrimSpace(t))
	}
	return res
}

func parseADR(adrPath string) (*ADR, error) {

	body, err := ioutil.ReadFile(adrPath)
	if err != nil {
		panic(err)
	}

	adr := ADR{
		Meta: ADRMeta{
			Path: adrPath,
		},
	}

	base := strings.TrimSuffix(path.Base(adrPath), path.Ext(adrPath))

	parts := strings.Split(base, "-")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid filename %s in %s", base, adrPath)
	}

	idx, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid file sequence %s in %s", parts[0], adrPath)
	}

	adr.Meta.Index = idx

	adr.Heading = extractHeader(string(body))

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	isMetaDataStart := false
	metaMap := make(map[string]string)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "|Metadata") {
			isMetaDataStart = true
			continue
		}

		if isMetaDataStart && strings.HasPrefix(line, "|===") {
			isMetaDataStart = false
		}

		if isMetaDataStart && strings.Contains(line, "|") {
			parts := strings.Split(strings.TrimSpace(line), "|")
			key := strings.TrimSpace(parts[1])
			value := strings.TrimSpace(parts[2])
			metaMap[key] = value
			//log.Printf("Key %s, Value %s", key, value)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading file ", err)
	}

	for key, value := range metaMap {
		switch key {
		case "Date":
			layout := "02-01-2006"
			t, err := time.Parse(layout, value)
			if err != nil {
				return nil, fmt.Errorf("invalid date format, not DD-MM-YYYY: %s", err)
			}
			adr.Meta.Date = t
		case "Author":
			adr.Meta.Authors = parseCommaList(value)
		case "Status":
			adr.Meta.Status = value
		case "Tags":
			adr.Meta.Tags = parseCommaList(value)
		default:
			log.Println("Unexpected meta key", key)
		}

		//log.Printf("Key %s, Value %s", key, value)
	}

	if adr.Meta.Index == 0 {
		return nil, fmt.Errorf("invalid ADR Index in %s", adr.Meta.Path)
	}
	if adr.Meta.Date.IsZero() {
		return nil, fmt.Errorf("date is required in %s", adr.Meta.Path)
	}
	if !isValidStatus(adr.Meta.Status) {
		return nil, fmt.Errorf("invalid status %q, must be one of: %s in %s", adr.Meta.Status, strings.Join(validStatus, ", "), adr.Meta.Path)
	}
	if len(adr.Meta.Authors) == 0 {
		return nil, fmt.Errorf("authors is required in %s", adr.Meta.Path)
	}
	if len(adr.Meta.Tags) == 0 {
		return nil, fmt.Errorf("tags is required in %s", adr.Meta.Path)
	}

	return &adr, nil
}

func isValidStatus(status string) bool {
	for _, s := range validStatus {
		if status == s {
			return true
		}
	}

	return false
}

func verifyUniqueIndexes(adrs []*ADR) error {
	indexes := map[int]string{}
	for _, a := range adrs {
		path, ok := indexes[a.Meta.Index]
		if ok {
			return fmt.Errorf("duplicate index %d, conflict between %s and %s", a.Meta.Index, a.Meta.Path, path)
		}
		indexes[a.Meta.Index] = a.Meta.Path
	}

	return nil
}

func renderIndexes(adrs []*ADR) error {
	tags := map[string]int{}
	for _, adr := range adrs {
		for _, tag := range adr.Meta.Tags {
			tags[tag] = 1
		}
	}

	tagsList := []string{}
	for k := range tags {
		tagsList = append(tagsList, k)
	}
	sort.Strings(tagsList)

	type tagAdrs struct {
		Tag  string
		Adrs []*ADR
	}

	renderList := []tagAdrs{}

	for _, tag := range tagsList {
		matched := []*ADR{}
		for _, adr := range adrs {
			for _, mt := range adr.Meta.Tags {
				if tag == mt {
					matched = append(matched, adr)
				}
			}
		}

		sort.Slice(matched, func(i, j int) bool {
			return matched[i].Meta.Index < matched[j].Meta.Index
		})

		renderList = append(renderList, tagAdrs{Tag: tag, Adrs: matched})
	}

	funcMap := template.FuncMap{
		"join": func(i []string) string {
			return strings.Join(i, ", ")
		},
		"title": func(i string) string {
			return strings.Title(i)
		},
	}

	readme, err := template.New(".readme.templ").Funcs(funcMap).ParseFiles(".readme.templ")
	if err != nil {
		return err
	}
	err = readme.Execute(os.Stdout, renderList)
	if err != nil {
		return err
	}
	return nil
}

func extractHeader(asciidocContent string) string {
	// Regular expression to match AsciiDoc headers
	headerRegex := regexp.MustCompile(`^=\s.*`)

	// Find the first match
	match := headerRegex.FindStringSubmatch(asciidocContent)
	// Check if a match is found
	if len(match) >= 1 {
		return strings.TrimPrefix(match[0], "= ")
	}

	// Return an empty string if no header is found
	return ""
}

func main() {
	dir, err := ioutil.ReadDir("adr")
	if err != nil {
		panic(err)
	}

	adrs := []*ADR{}

	for _, mdf := range dir {
		if mdf.IsDir() {
			continue
		}

		if path.Ext(mdf.Name()) != ".adoc" {
			continue
		}

		adr, err := parseADR(path.Join("adr", mdf.Name()))
		if err != nil {
			panic(err)
		}

		adrs = append(adrs, adr)
	}

	err = verifyUniqueIndexes(adrs)
	if err != nil {
		panic(err)
	}

	err = renderIndexes(adrs)
	if err != nil {
		panic(err)
	}
}
