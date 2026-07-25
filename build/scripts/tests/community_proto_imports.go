// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	protoModulePrefix = "github.com/erda-project/erda-proto-go/"
	accesskeyImport   = protoModulePrefix + "core/services/authentication/credentials/accesskey/pb"
)

var protoPathPattern = regexp.MustCompile(`github\.com/erda-project/erda-proto-go/[A-Za-z0-9_./-]+`)

func main() {
	var sourceRoot string
	var generatedRoot string
	var expectedActive int
	var expectedLexical int
	flag.StringVar(&sourceRoot, "source-root", ".", "Erda source tree")
	flag.StringVar(&generatedRoot, "generated-root", "api/proto-go", "generated erda-proto-go tree")
	flag.IntVar(&expectedActive, "expected-active", 168, "expected unique Go AST imports")
	flag.IntVar(&expectedLexical, "expected-lexical", 169, "expected unique lexical paths")
	flag.Parse()

	active, lexical, comments, err := collectSourcePaths(sourceRoot)
	if err != nil {
		fatal(err)
	}
	commentOnly := setDifference(lexical, active)
	if len(active) != expectedActive {
		fatalf("active Go AST imports: got %d, want %d", len(active), expectedActive)
	}
	if len(lexical) != expectedLexical {
		fatalf("lexical proto paths: got %d, want %d", len(lexical), expectedLexical)
	}
	if len(commentOnly) != 1 || commentOnly[0] != accesskeyImport {
		fatalf("comment-only audit: got %v, want [%s]", commentOnly, accesskeyImport)
	}
	if comments[accesskeyImport] != 2 {
		fatalf("comment-only accesskey occurrences: got %d, want 2", comments[accesskeyImport])
	}

	missing, resolved, err := resolveGeneratedPackages(generatedRoot, active)
	if err != nil {
		fatal(err)
	}
	fmt.Printf(
		"active=%d lexical=%d comment_only=%d resolved=%d missing=%d\n",
		len(active), len(lexical), len(commentOnly), resolved, len(missing),
	)
	if len(missing) != 0 {
		for _, importPath := range missing {
			fmt.Println(importPath)
		}
		os.Exit(1)
	}
}

func collectSourcePaths(root string) (map[string]struct{}, map[string]struct{}, map[string]int, error) {
	active := make(map[string]struct{})
	lexical := make(map[string]struct{})
	comments := make(map[string]int)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == ".git" || relative == "api/proto-go" ||
				relative == "vendor" || relative == "build/scripts/tests" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range protoPathPattern.FindAllString(string(content), -1) {
			lexical[match] = struct{}{}
		}
		parsed, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, importSpec := range parsed.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if strings.HasPrefix(importPath, protoModulePrefix) {
				active[importPath] = struct{}{}
			}
		}
		for _, commentGroup := range parsed.Comments {
			for _, comment := range commentGroup.List {
				for _, match := range protoPathPattern.FindAllString(comment.Text, -1) {
					comments[match]++
				}
			}
		}
		return nil
	})
	return active, lexical, comments, err
}

func resolveGeneratedPackages(root string, active map[string]struct{}) ([]string, int, error) {
	var missing []string
	resolved := 0
	for importPath := range active {
		relative := strings.TrimPrefix(importPath, protoModulePrefix)
		directory := filepath.Join(root, filepath.FromSlash(relative))
		entries, err := os.ReadDir(directory)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, importPath)
				continue
			}
			return nil, 0, err
		}
		foundGo := false
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				foundGo = true
				break
			}
		}
		if foundGo {
			resolved++
		} else {
			missing = append(missing, importPath)
		}
	}
	sort.Strings(missing)
	return missing, resolved, nil
}

func setDifference(left, right map[string]struct{}) []string {
	var result []string
	for value := range left {
		if _, ok := right[value]; !ok {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func fatalf(format string, args ...any) {
	fatal(fmt.Errorf(format, args...))
}

var _ ast.Node
