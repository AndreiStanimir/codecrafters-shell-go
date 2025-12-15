package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	// Don't reassign wordsChan here!
	if ind := strings.Index(input, "'"); ind == -1 {
		for word := range strings.SplitSeq(standardizeSpaces(input), " ") {
			wordsChan <- Word{
				text:  word,
				typee: Normal,
			}
		}
		return
	}
	outside := true
	for len(input) > 0 {
		index := strings.Index(input, "'")
		if index == -1 {
			// Send any remaining text before returning
			if len(input) > 0 && outside {
				wordsChan <- Word{
					text:  input,
					typee: Normal,
				}
			}
			return
		}

		// Send text before the quote if we're outside quotes
		if index > 0 && outside {
			wordsChan <- Word{
				text:  input[:index],
				typee: Normal,
			}
		}

		index2 := strings.Index(input[index+1:], "'")
		if index2 == -1 {
			// No closing quote found
			return
		}

		typ := Normal
		if outside {
			typ = SingleQuote
		} else {
			typ = Normal
		}

		// Send the text between quotes
		wordsChan <- Word{
			text:  input[index+1 : index+1+index2],
			typee: typ,
		}

		outside = !outside
		input = input[index+1+index2+1:]
	}
	return
}

func ChanToSlice(ch interface{}) interface{} {
	chv := reflect.ValueOf(ch)
	slv := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(ch).Elem()), 0, 0)
	for {
		v, ok := chv.Recv()
		if !ok {
			return slv.Interface()
		}
		slv = reflect.Append(slv, v)
	}
}

func readEverythingFromChannel(ch chan Word) []Word {
	somethings := []Word{}
	for s := range ch {
		somethings = append(somethings, s)
	}
	return somethings
}

func splitWithQuotes(s string) []string {
	var result []string
	var current string
	inQuote := false
	inDoubleQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && (!inQuote && !inDoubleQuote) {
			current += string(s[i+1])
			i++
		} else if s[i] == '\\' && inQuote && !inDoubleQuote {
			current += string(s[i : i+2])
			i++
		} else if s[i] == '\\' && inDoubleQuote {
			switch s[i+1] {
			case '"', '\\':
				current += string(s[i+1])
				i++
			default:
				current += s[i : i+2]
				i++
			}
			// default:
		} else if s[i] == '"' {
			if inQuote {
				// literal double quote inside single quotes
				current += `"`
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		} else if s[i] == '\'' {
			if !inDoubleQuote {
				inQuote = !inQuote
			} else {
				current += string(s[i])
			}
			// current += string(s[i])
		} else if s[i] == ' ' && (!inQuote && !inDoubleQuote) {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func main() {
	commands := map[string]func([]string, io.Writer) bool{
		"exit": func(_ []string, _ io.Writer) bool {
			os.Exit(0)
			return true
		},

		"echo": func(args []string, out io.Writer) bool {
			fmt.Fprintln(out, strings.Join(args, " "))
			return true
		},

		"pwd": func(_ []string, out io.Writer) bool {
			dir, err := os.Getwd()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return true
			}
			fmt.Fprintln(out, dir)
			return true
		},

		"cd": func(args []string, _ io.Writer) bool {
			if len(args) == 0 {
				return true
			}
			path := args[0]
			if path == "~" {
				path = os.Getenv("HOME")
			}
			if err := os.Chdir(path); err != nil {
				fmt.Fprintf(os.Stdout, "cd: %s: No such file or directory\n", path)
			}
			return true
		},

		"type": func(args []string, out io.Writer) bool {
			if len(args) == 0 {
				return true
			}
			builtins := map[string]bool{
				"exit": true, "echo": true, "type": true, "pwd": true, "cd": true,
			}
			if builtins[args[0]] {
				fmt.Fprintf(out, "%s is a shell builtin\n", args[0])
				return true
			}

			PATH := os.Getenv("PATH")
			for _, p := range strings.Split(PATH, ":") {
				entries, err := os.ReadDir(p)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if e.Name() == args[0] {
						info, _ := e.Info()
						if IsExecAny(info.Mode()) {
							fmt.Fprintf(out, "%s is %s\n", args[0], filepath.Join(p, args[0]))
							return true
						}
					}
				}
			}
			fmt.Fprintf(out, "%s: not found\n", args[0])
			return true
		},
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Fprint(os.Stdout, "$ ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// --- redirection parsing ---
		redirectFile := ""
		redirectErr := false
		if i := strings.LastIndex(line, "1>"); i != -1 {
			redirectFile = strings.TrimSpace(line[i+2:])
			line = strings.TrimSpace(line[:i])
		} else if i := strings.LastIndex(line, "2>"); i != -1 {
			redirectErr = true
			redirectFile = strings.TrimSpace(line[i+2:])
			line = strings.TrimSpace(line[:i])
		} else if i := strings.LastIndex(line, ">"); i != -1 {
			redirectFile = strings.TrimSpace(line[i+1:])
			line = strings.TrimSpace(line[:i])
		}

		// --- stdout selection ---
		out := io.Writer(os.Stdout)
		outErr := io.Writer(os.Stderr)
		var outfile *os.File
		if redirectFile != "" {
			f, err := os.Create(redirectFile)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			outfile = f
			if redirectErr {
				outErr = outfile
			} else {
				out = outfile
			}
		}

		words := splitWithQuotes(line)
		cmd := words[0]
		args := words[1:]

		// --- builtin ---
		if fn, ok := commands[cmd]; ok {
			fn(args, out)
		} else {
			if _, err := exec.LookPath(cmd); err != nil {
				fmt.Fprintf(out, "%s: command not found\n", cmd)
			} else {
				c := exec.Command(cmd, args...)
				c.Stdout = out
				c.Stderr = outErr
				c.Stdin = os.Stdin
				_ = c.Run()
			}
		}

		if outfile != nil {
			outfile.Close()
		}
	}
}
