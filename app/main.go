package main

import (
	"bufio"
	"fmt"
	"log"
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
	for {
		_, err := fmt.Fprint(os.Stdout, "$ ")
		if err != nil {
			return
		}
		inputOriginal, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		input := strings.ReplaceAll(inputOriginal, "''", "")
		inputTrimmed := strings.TrimSpace(input)
		words := splitWithQuotes(inputTrimmed)
		command := words[0]

		// fmt.Fprintf(os.Stdout,"command::::*%s",command) )
		wordsChan := make(chan Word)
		go func() {
			SanitizeSingleQotesChannel(inputTrimmed[len(command):], wordsChan)
			close(wordsChan)
		}()
		rest := words[1:]

		switch command {
		case "exit":
			return
		case "echo":
			result := strings.Join(rest, " ")
			fmt.Fprintf(os.Stdout, "%s\n", result)
		case "type":
			switch rest[0] {
			case "exit", "echo", "type", "pwd", "cd":
				fmt.Fprintf(os.Stdout, "%s is a shell builtin\n", rest[0])
			default:
				PATH := os.Getenv("PATH")
				found := false
				for path := range strings.SplitSeq(PATH, ":") {
					files, err := os.ReadDir(path)
					if err != nil {
						continue
					}
					for _, f := range files {
						if !f.IsDir() && f.Name() == rest[0] {
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
		case "pwd":
			dir, err := os.Getwd()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(dir)
		case "cd":
			if rest[0] == "~" {
				rest[0] = os.Getenv("HOME")
			}
			if _, err := os.Stat(rest[0]); os.IsNotExist(err) {
				fmt.Fprintf(os.Stdout, "cd: %s: No such file or directory\n", rest[0])
			}
			os.Chdir(rest[0])
		default:
			_, err := exec.LookPath(command)
			if err != nil {
				fmt.Fprintf(os.Stdout, "%s: command not found\n", command)
				continue
			}
			cmd := exec.Command(command, rest...)

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			_ = cmd.Run()
		}
	}
}
