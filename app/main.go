package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

func SanitizeSingleQotesChannel(input string, wordsChan chan Word) {
	if !strings.Contains(input, "'") {
		for _, word := range strings.Split(standardizeSpaces(input), " ") {
			if word != "" {
				wordsChan <- Word{text: word, typee: Normal}
			}
		}
		return
	}

	outside := true
	for len(input) > 0 {
		i := strings.Index(input, "'")
		if i == -1 {
			if outside && strings.TrimSpace(input) != "" {
				wordsChan <- Word{text: input, typee: Normal}
			}
			return
		}

		if i > 0 && outside {
			wordsChan <- Word{text: input[:i], typee: Normal}
		}

		j := strings.Index(input[i+1:], "'")
		if j == -1 {
			return
		}

		wordsChan <- Word{
			text:  input[i+1 : i+1+j],
			typee: SingleQuote,
		}

		outside = !outside
		input = input[i+1+j+1:]
	}
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
				// single quote inside double quotes is literal
				cur += "'"
			}

		case '"':
			if !inSQ {
				inDQ = !inDQ
			} else {
				// double quote inside single quotes is literal
				cur += `"`
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
			// Improved escaping rules:
			// - inside single quotes -> backslash is literal
			// - inside double quotes -> backslash only escapes: " $ ` \ and newline
			// - outside quotes -> backslash escapes next char (append next char)
			if inSQ {
				// literal backslash inside single quotes
				cur += "\\"
			} else if inDQ {
				if i+1 < len(s) {
					next := s[i+1]
					// In double quotes, backslash only escapes these specific characters
					if next == '"' || next == '$' || next == '`' || next == '\\' || next == '\n' {
						cur += string(next)
						i++
					} else {
						// For other characters, backslash is literal
						cur += "\\"
					}
				} else {
					// trailing backslash — keep it
					cur += "\\"
				}
			} else {
				// outside quotes: escape next char
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
	// Try as-is first
	if p, e := exec.LookPath(name); e == nil {
		return p, name, nil
	}

	// Try unescaped variants (common cases from tests)
	tryVariants := []string{
		strings.ReplaceAll(name, `\'`, `'`),
		strings.ReplaceAll(name, `\"`, `"`),
		strings.ReplaceAll(name, `\\`, `\`),
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

	// As a last resort, search PATH entries for a name match (literal comparison).
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
			// also try unescaped forms
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

func notFound(string) []string {
	fmt.Print("\x07")
	return make([]string, 0)
}

func main() {
	rl, _ := readline.NewEx(&readline.Config{
		Prompt: "$ ",
		AutoComplete: readline.NewPrefixCompleter(
			readline.PcItem("echo"),
			readline.PcItem("exit"),
			readline.PcItem("cd"),
			readline.PcItem("pwd"),
			readline.PcItem("type"),
			readline.PcItemDynamic(notFound),
		),
	})

	for {
		line, err := rl.Readline()
		if err != nil {
			return
		}

		input := strings.TrimSpace(strings.ReplaceAll(line, "''", ""))
		if input == "" {
			continue
		}

		/* ---------- redirection parsing (supports append) ---------- */
		redirectFile := ""
		redirectStdErr := false
		appendMode := false

		// Check longer tokens first so we remove the correct characters from input.
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
				// overwrite/truncate
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
				"exit": true, "echo": true, "cd": true, "pwd": true, "type": true,
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
			// If exePath matches the original command name (no special path), just use cmd
			if exePath != "" && exePath != cmd {
				c = exec.Command(exePath, args...)
				// ensure the executed program sees the "unescaped" name as argv[0]
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
