package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: community_proto_tree_hash.go GENERATED_ROOT")
		os.Exit(2)
	}
	root := os.Args[1]
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no generated Go files in %s\n", root)
		os.Exit(1)
	}
	sort.Strings(files)
	tree := sha256.New()
	for _, relative := range files {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fileHash := sha256.Sum256(content)
		fmt.Fprintf(tree, "%s\x00%s\n", relative, hex.EncodeToString(fileHash[:]))
	}
	fmt.Printf("%x  files=%d\n", tree.Sum(nil), len(files))
}
