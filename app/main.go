package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	for true {
		fmt.Fprint(os.Stdout, "$ ")
		command, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		command = strings.TrimRight(command, "\n")
		if command == "exit" {
			return
		}
		fmt.Fprintf(os.Stdout, "%s: command not found", command)
		fmt.Fprint(os.Stdout, "\n")
	}
}
