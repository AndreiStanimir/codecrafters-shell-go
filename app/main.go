package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IsExecAny(mode os.FileMode) bool {
	return mode&0o111 != 0
}

func FilePathWalkDir(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func main() {
	for {
		_, err := fmt.Fprint(os.Stdout, "$ ")
		if err != nil {
			return
		}
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		words := strings.Split(strings.TrimSpace(input), " ")
		// command = strings.TrimRight(command, "\n")
		command := words[0]
		// fmt.Fprintf(os.Stderr, "You entered: %s\n", command)
		rest := words[1:]
		switch command {
		case "exit":
			return
		case "echo":
			fmt.Fprintf(os.Stdout, "%s\n", strings.Join(rest, " "))
		case "type":
			switch rest[0] {
			case "exit", "echo", "type":
				fmt.Fprintf(os.Stdout, "%s is a shell builtin\n", rest[0])
			default:
				PATH := os.Getenv("PATH")
				found := false
				for path := range strings.SplitSeq(PATH, ":") {
					// fmt.Fprintf(os.Stdout, "Searching in %s\n", path)

					files, err := os.ReadDir(path)
					if err != nil {
						// fmt.Fprintf(os.Stdout, "%s error: %v\n", path, err)
						continue
					}
					for _, f := range files {
						// fmt.Fprintf(os.Stdout, "Checking file: %s\n", f.Name())
						if !f.IsDir() && f.Name() == rest[0] {
							// fmt.Fprintln(os.Stdout, "file: %s", f.Name())
							info, _ := f.Info()
							if IsExecAny(info.Mode()) {
								fmt.Fprintf(os.Stdout, "%s is %s\n", rest[0], filepath.Join(path, rest[0]))
								found = true
								break
							}
						}
					}
					if found {
						break
					}
				}
				if !found {
					fmt.Fprintf(os.Stdout, "%s: not found\n", rest[0])
				}
			}
		default:
			fmt.Fprintf(os.Stdout, "%s: command not found", command)
			fmt.Fprint(os.Stdout, "\n")

		}
	}
}
