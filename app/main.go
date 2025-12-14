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

	"github.com/samber/lo"
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
	wordsChan = make(chan Word)
	if ind := strings.Index(input, "'"); ind == -1 {
		return
	}
	outside := true
	for len(input) > 0 {
		index := strings.Index(input, "'")
		if index == -1 {
			return
		}
		index2 := strings.Index(input[index+1:], "'")

		typ := Normal
		if outside {
			typ = Normal
		} else {
			typ = SingleQuote
		}

		if index+1 < index2 {
			wordsChan <- Word{
				text:  input[index+1 : index2],
				typee: typ,
			}
		}
		outside = !outside
		input = input[index2+1:]
	}
	return
	// return strings.ReplaceAll(input, "'", "")
}

func SanitizeSingleQuotes(input string) chan string {
	wordsChan := make(chan string)
	if ind := strings.Index(input, "'"); ind == -1 {
		return wordsChan
	}
	for len(input) > 0 {
		index := strings.Index(input, "'")
		if index == -1 {
			return wordsChan
		}
		index2 := strings.Index(input[index+1:], "'")
		wordsChan <- input[index+1 : index2]
		input = input[index2+1:]
	}
	return wordsChan
	// return strings.ReplaceAll(input, "'", "")
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

func main() {
	for {
		_, err := fmt.Fprint(os.Stdout, "$ ")
		if err != nil {
			return
		}
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		input = strings.ReplaceAll(input, "''", "")
		// input = strings.ReplaceAll(input, "'", "\\'")
		// wordsQuotes := strings.Split(input, "'")
		// if len(wordsQuotes)>1{

		//}
		inputTrimmed := strings.TrimSpace(input)
		words := strings.Split(inputTrimmed, " ")
		// command = strings.TrimRight(command, "\n")
		command := words[0]
		// foralsemt.Fprintf(os.Stderr, "You entered: %s\n", command)
		wordsChan := make(chan Word)
		go func() {
			SanitizeSingleQotesChannel(inputTrimmed[len(command):], wordsChan)
			close(wordsChan)
		}()
		rest := readEverythingFromChannel(wordsChan)
		// rest := ChanToSlice(wordsChan).([]Word)
		switch command {
		case "exit":
			return
		case "echo":
			result := ""
			for _, word := range rest {
				switch word.typee {
				case Normal:
					result += standardizeSpaces(word.text)
				case SingleQuote:
					result += word.text
				}
			}
			fmt.Fprintf(os.Stdout, "%s\n", result)
		case "type":
			switch rest[0].text {
			case "exit", "echo", "type", "pwd", "cd":
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
						if !f.IsDir() && f.Name() == rest[0].text {
							// fmt.Fprintln(os.Stdout, "file: %s", f.Name())
							info, _ := f.Info()
							if IsExecAny(info.Mode()) {
								fmt.Fprintf(os.Stdout, "%s is %s\n", rest[0], filepath.Join(path, rest[0].text))
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
			if rest[0].text == "~" {
				rest[0].text = os.Getenv("HOME")
			}
			if _, err := os.Stat(rest[0].text); os.IsNotExist(err) {
				fmt.Fprintf(os.Stdout, "cd: %s: No such file or directory\n", rest[0])
			}
			os.Chdir(rest[0].text)
		default:
			_, err := exec.LookPath(command)
			if err != nil {
				fmt.Fprintf(os.Stdout, "%s: command not found\n", command)
				continue
			}

			cmd := exec.Command(command, lo.Map(rest, func(item Word, index int) string { return item.text })...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			_ = cmd.Run() // ignore error, do not exit shell
		}
	}
}
