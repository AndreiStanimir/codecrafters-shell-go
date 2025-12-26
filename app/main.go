package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chzyer/readline"
)

func IsExecAny(mode os.FileMode) bool {
	return mode&0o111 != 0
}

func standardizeSpaces(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

type WordType int

const (
	Normal WordType = iota
	SingleQuote
	DoubleQuote
)

type Word struct {
	text  string
	typee WordType
}

func splitWithQuotes(s string) []string {
	var res []string
	var cur string
	inSQ, inDQ := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDQ {
				inSQ = !inSQ
			} else {
				cur += "'"
			}
		case '"':
			if !inSQ {
				inDQ = !inDQ
			} else {
				cur += "\""
			}
		case ' ':
			if !inSQ && !inDQ {
				if cur != "" {
					res = append(res, cur)
					cur = ""
				}
			} else {
				cur += " "
			}
		case '\\':
			if inSQ {
				cur += "\\"
			} else if inDQ {
				if i+1 < len(s) {
					next := s[i+1]
					if next == '"' || next == '$' || next == '`' || next == '\\' || next == '\n' {
						cur += string(next)
						i++
					} else {
						cur += "\\"
					}
				} else {
					cur += "\\"
				}
			} else {
				if i+1 < len(s) {
					cur += string(s[i+1])
					i++
				}
			}
		default:
			cur += string(s[i])
		}
	}
	if cur != "" {
		res = append(res, cur)
	}
	return res
}

func findExecutableWithUnescape(name string) (exePath string, argv0 string, err error) {
	if p, e := exec.LookPath(name); e == nil {
		return p, name, nil
	}

	tryVariants := []string{
		strings.ReplaceAll(name, "\\'", "'"),
		strings.ReplaceAll(name, "\\\"", "\""),
		strings.ReplaceAll(name, "\\\\", "\\"),
	}
	seen := map[string]bool{}
	for _, v := range tryVariants {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		if p, e := exec.LookPath(v); e == nil {
			return p, v, nil
		}
	}

	for _, d := range strings.Split(os.Getenv("PATH"), ":") {
		if d == "" {
			continue
		}
		ents, _ := os.ReadDir(d)
		for _, en := range ents {
			if en.Name() == name {
				p := filepath.Join(d, en.Name())
				if info, _ := en.Info(); info != nil && IsExecAny(info.Mode()) {
					return p, name, nil
				}
			}
			for _, v := range tryVariants {
				if en.Name() == v {
					p := filepath.Join(d, en.Name())
					if info, _ := en.Info(); info != nil && IsExecAny(info.Mode()) {
						return p, v, nil
					}
				}
			}
		}
	}
	return "", "", fmt.Errorf("not found")
}

func getExecutablesInPath() []string {
	pathEnv, ok := os.LookupEnv("PATH")
	if !ok {
		return nil
	}
	var executables []string
	seen := make(map[string]bool)
	for _, dir := range strings.Split(pathEnv, ":") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0o111 != 0 {
				name := entry.Name()
				if !seen[name] {
					seen[name] = true
					executables = append(executables, name)
				}
			}
		}
	}
	sort.Strings(executables)
	return executables
}

func main() {
	var lastPrefix string
	var tabCount int
	var rlInstance *readline.Instance

	completer := readline.NewPrefixCompleter(
		readline.PcItem("echo"),
		readline.PcItem("exit"),
		readline.PcItem("cd"),
		readline.PcItem("pwd"),
		readline.PcItem("type"),
		readline.PcItemDynamic(func(input string) []string {
			// Get the current prefix (last word being typed)
			prefix := input
			if idx := strings.LastIndex(input, " "); idx != -1 {
				prefix = input[idx+1:]
			}

			// If prefix changed, reset tab count
			if prefix != lastPrefix {
				lastPrefix = prefix
				tabCount = 0
			}

			tabCount++

			// Get all executables and filter by prefix
			allExecs := getExecutablesInPath()
			var matches []string
			for _, exec := range allExecs {
				if strings.HasPrefix(exec, prefix) {
					matches = append(matches, exec)
				}
			}
			if len(matches) == 1 {
				return matches[:]
			}

			// First tab: ring bell, return nothing
			if tabCount == 1 {
				fmt.Print("\x07")
				return []string{}
			}

			// Second tab: print matches with two spaces, then refresh prompt
			if tabCount >= 2 {
				tabCount = 0 // Reset for next cycle
				if len(matches) > 0 {
					fmt.Println()
					fmt.Println(strings.Join(matches, "  "))
					if rlInstance != nil {
						rlInstance.Refresh()
					}
				}
				return []string{}
			}

			return []string{}
		}),
	)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "$ ",
		AutoComplete: completer,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer rl.Close()

	// Store reference for completer to use
	rlInstance = rl

	for {
		line, err := rl.Readline()
		if err != nil {
			return
		}

		// Reset tab state after each command
		lastPrefix = ""
		tabCount = 0

		input := strings.TrimSpace(strings.ReplaceAll(line, "''", ""))
		if input == "" {
			continue
		}

		/* ---------- redirection parsing (supports append) ---------- */
		redirectFile := ""
		redirectStdErr := false
		appendMode := false

		if i := strings.LastIndex(input, "2>>"); i != -1 {
			redirectStdErr = true
			appendMode = true
			redirectFile = strings.TrimSpace(input[i+3:])
			input = strings.TrimSpace(input[:i])
		} else if i := strings.LastIndex(input, "1>>"); i != -1 {
			appendMode = true
			redirectFile = strings.TrimSpace(input[i+3:])
			input = strings.TrimSpace(input[:i])
		} else if i := strings.LastIndex(input, ">>"); i != -1 {
			appendMode = true
			redirectFile = strings.TrimSpace(input[i+2:])
			input = strings.TrimSpace(input[:i])
		} else if i := strings.LastIndex(input, "2>"); i != -1 {
			redirectStdErr = true
			redirectFile = strings.TrimSpace(input[i+2:])
			input = strings.TrimSpace(input[:i])
		} else if i := strings.LastIndex(input, "1>"); i != -1 {
			redirectFile = strings.TrimSpace(input[i+2:])
			input = strings.TrimSpace(input[:i])
		} else if i := strings.LastIndex(input, ">"); i != -1 {
			redirectFile = strings.TrimSpace(input[i+1:])
			input = strings.TrimSpace(input[:i])
		}

		words := splitWithQuotes(input)
		if len(words) == 0 {
			continue
		}

		cmd := words[0]
		args := []string{}
		if len(words) > 1 {
			args = words[1:]
		}

		out := io.Writer(os.Stdout)
		outErr := io.Writer(os.Stderr)
		var outfile *os.File

		if redirectFile != "" {
			var f *os.File
			var ferr error
			if appendMode {
				f, ferr = os.OpenFile(redirectFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			} else {
				f, ferr = os.Create(redirectFile)
			}
			if ferr != nil {
				fmt.Fprintln(os.Stderr, ferr)
				continue
			}
			outfile = f
			if redirectStdErr {
				outErr = f
			} else {
				out = f
			}
		}

		/* ---------- builtins ---------- */
		switch cmd {
		case "exit":
			if outfile != nil {
				outfile.Close()
			}
			return
		case "echo":
			fmt.Fprintln(out, strings.Join(args, " "))
		case "pwd":
			dir, err := os.Getwd()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Fprintln(out, dir)
		case "cd":
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			if path == "~" {
				path = os.Getenv("HOME")
			}
			if err := os.Chdir(path); err != nil {
				fmt.Fprintf(out, "cd: %s: No such file or directory\n", path)
			}
		case "type":
			if len(args) == 0 {
				break
			}
			builtins := map[string]bool{
				"exit": true,
				"echo": true,
				"cd":   true,
				"pwd":  true,
				"type": true,
			}
			if builtins[args[0]] {
				fmt.Fprintf(out, "%s is a shell builtin\n", args[0])
				break
			}
			found := false
			for _, p := range strings.Split(os.Getenv("PATH"), ":") {
				entries, _ := os.ReadDir(p)
				for _, e := range entries {
					if e.Name() == args[0] {
						info, _ := e.Info()
						if IsExecAny(info.Mode()) {
							fmt.Fprintf(out, "%s is %s\n", args[0], filepath.Join(p, args[0]))
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
				fmt.Fprintf(out, "%s: not found\n", args[0])
			}

		/* ---------- external ---------- */
		default:
			exePath, argv0, e := findExecutableWithUnescape(cmd)
			if e != nil {
				fmt.Fprintf(out, "%s: command not found\n", cmd)
				break
			}

			var c *exec.Cmd
			if exePath != "" && exePath != cmd {
				c = exec.Command(exePath, args...)
				c.Args[0] = argv0
			} else {
				c = exec.Command(cmd, args...)
			}

			c.Stdout = out
			c.Stderr = outErr
			c.Stdin = os.Stdin
			_ = c.Run()
		}

		if outfile != nil {
			outfile.Close()
		}
	}
}
